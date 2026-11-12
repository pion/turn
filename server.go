// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package turn contains the public API for pion/turn, a toolkit for building TURN clients and servers
package turn

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v4"
	"github.com/pion/logging"
	"github.com/pion/turn/v6/internal/allocation"
	"github.com/pion/turn/v6/internal/proto"
	"github.com/pion/turn/v6/internal/server"
)

const (
	defaultInboundMTU      = 1600
	defaultDTLSIdleTimeout = time.Minute
	handshakeTimeout       = 10 * time.Second
)

// Server is an instance of the Pion TURN Server.
type Server struct {
	log                 logging.LeveledLogger
	authHandler         AuthHandler
	quotaHandler        QuotaHandler
	realm               string
	channelBindTimeout  time.Duration
	permissionTimeout   time.Duration
	allocationLifetime  time.Duration
	dtlsIdleTimeout     time.Duration
	strictAddressFamily bool
	nonceHash           server.NonceManager
	eventHandler        EventHandler

	packetConnConfigs  []PacketConnConfig
	listenerConfigs    []ListenerConfig
	allocationManagers []*allocation.Manager
	inboundMTU         int
}

// NewServer creates the Pion TURN server.
func NewServer(config ServerConfig) (*Server, error) { //nolint:gocognit,cyclop
	if err := config.validate(); err != nil {
		return nil, err
	}

	loggerFactory := config.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = logging.NewDefaultLoggerFactory()
	}

	mtu := defaultInboundMTU
	if config.InboundMTU != 0 {
		mtu = config.InboundMTU
	}

	nonceHash, err := server.NewShortNonceHash(0)
	if err != nil {
		return nil, err
	}

	server := &Server{
		log:                 loggerFactory.NewLogger("turn"),
		authHandler:         config.AuthHandler,
		quotaHandler:        config.QuotaHandler,
		realm:               config.Realm,
		channelBindTimeout:  config.ChannelBindTimeout,
		permissionTimeout:   config.PermissionTimeout,
		allocationLifetime:  config.AllocationLifetime,
		dtlsIdleTimeout:     config.DTLSIdleTimeout,
		strictAddressFamily: config.StrictAddressFamily,
		packetConnConfigs:   config.PacketConnConfigs,
		listenerConfigs:     config.ListenerConfigs,
		nonceHash:           nonceHash,
		inboundMTU:          mtu,
		eventHandler:        config.EventHandler,
	}

	if server.dtlsIdleTimeout == 0 {
		server.dtlsIdleTimeout = defaultDTLSIdleTimeout
	}
	if server.channelBindTimeout == 0 {
		server.channelBindTimeout = proto.DefaultLifetime
	}
	if server.permissionTimeout == 0 {
		server.permissionTimeout = allocation.DefaultPermissionTimeout
	}
	if server.allocationLifetime == 0 {
		server.allocationLifetime = proto.DefaultLifetime
	}

	for _, cfg := range server.packetConnConfigs {
		am, err := server.createAllocationManager(cfg.RelayAddressGenerator, cfg.PermissionHandler)
		if err != nil {
			return nil, fmt.Errorf("failed to create AllocationManager: %w", err)
		}

		go func(cfg PacketConnConfig, am *allocation.Manager) {
			server.readLoop(cfg.PacketConn, am, nil, nil)

			if err := am.Close(); err != nil {
				server.log.Errorf("Failed to close AllocationManager: %s", err)
			}
		}(cfg, am)
	}

	for _, cfg := range server.listenerConfigs {
		am, err := server.createAllocationManager(cfg.RelayAddressGenerator, cfg.PermissionHandler)
		if err != nil {
			return nil, fmt.Errorf("failed to create AllocationManager: %w", err)
		}

		go func(cfg ListenerConfig, am *allocation.Manager) {
			server.readListener(cfg.Listener, am)

			if err := am.Close(); err != nil {
				server.log.Errorf("Failed to close AllocationManager: %s", err)
			}
		}(cfg, am)
	}

	return server, nil
}

// AllocationCount returns the number of active allocations.
// It can be used to drain the server before closing.
func (s *Server) AllocationCount() int {
	allocs := 0
	for _, am := range s.allocationManagers {
		allocs += am.AllocationCount()
	}

	return allocs
}

// Close stops the TURN Server.
// It cleans up any associated state and closes all connections it is managing.
func (s *Server) Close() error {
	var errors []error

	for _, cfg := range s.packetConnConfigs {
		if err := cfg.PacketConn.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	for _, cfg := range s.listenerConfigs {
		if err := cfg.Listener.Close(); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	err := errFailedToClose
	for _, e := range errors {
		err = fmt.Errorf("%s; close error (%w) ", err, e) //nolint:errorlint
	}

	return err
}

func (s *Server) readListener(listener net.Listener, am *allocation.Manager) {
	var (
		wg      sync.WaitGroup
		connsMu sync.Mutex
		conns   = map[*net.Conn]struct{}{}
	)

	// Closing the listener leaves accepted connections open.
	defer func() {
		connsMu.Lock()
		for conn := range conns {
			_ = (*conn).Close()
		}
		connsMu.Unlock()

		wg.Wait()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.log.Debugf("Failed to accept: %s", err)

			return
		}

		entry := &conn
		connsMu.Lock()
		conns[entry] = struct{}{}
		connsMu.Unlock()

		wg.Add(1)
		go func() {
			defer wg.Done()

			s.serveConn(conn, am)

			connsMu.Lock()
			delete(conns, entry)
			connsMu.Unlock()
		}()
	}
}

// serveConn handles a connection accepted by a listener until it is closed.
func (s *Server) serveConn(conn net.Conn, am *allocation.Manager) {
	defer func() {
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, dtls.ErrConnClosed) {
			s.log.Errorf("Failed to close conn: %s", err)
		}
	}()

	fiveTuple := &allocation.FiveTuple{
		Protocol: allocation.UDP, // fixed UDP
		SrcAddr:  conn.RemoteAddr(),
		DstAddr:  conn.LocalAddr(),
	}

	var (
		packetConn net.PacketConn
		tlsState   *tls.ConnectionState
		dtlsState  *dtls.State
	)

	// Bound the handshake and populate peer certificates before authorization.
	switch typedConn := conn.(type) {
	case *tls.Conn:
		ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
		err := typedConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			s.log.Errorf("TLS handshake failed: %s", err)

			return
		}

		state := typedConn.ConnectionState()
		tlsState = &state
		packetConn = NewSTUNConn(conn)
	case *dtls.Conn:
		ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
		err := typedConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			s.log.Errorf("DTLS handshake failed: %s", err)

			return
		}

		if state, ok := typedConn.ConnectionState(); ok {
			dtlsState = &state
		}
		packetConn = &idleConn{
			PacketConn: NewDTLSConn(typedConn),
			timeout:    s.dtlsIdleTimeout,
			keepAlive: func() bool {
				return am.GetAllocation(fiveTuple) != nil
			},
		}
	default:
		packetConn = NewSTUNConn(conn)
	}

	s.readLoop(packetConn, am, tlsState, dtlsState)

	am.DeleteAllocation(fiveTuple)
}

// idleConn times out inactive connections unless keepAlive reports an active allocation.
type idleConn struct {
	net.PacketConn
	timeout   time.Duration
	keepAlive func() bool
}

func (c *idleConn) ReadFrom(p []byte) (int, net.Addr, error) {
	for {
		if err := c.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			return 0, nil, err
		}

		n, addr, err := c.PacketConn.ReadFrom(p)

		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() && c.keepAlive() {
			continue
		}

		return n, addr, err
	}
}

type nilAddressGenerator struct{}

func (n *nilAddressGenerator) Validate() error { return errRelayAddressGeneratorNil }

func (n *nilAddressGenerator) AllocatePacketConn(AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	return nil, nil, errRelayAddressGeneratorNil
}

func (n *nilAddressGenerator) AllocateListener(AllocateListenerConfig) (net.Listener, net.Addr, error) {
	return nil, nil, errRelayAddressGeneratorNil
}

func (n *nilAddressGenerator) AllocateConn(AllocateConnConfig) (net.Conn, error) {
	return nil, errRelayAddressGeneratorNil
}

func (s *Server) createAllocationManager(
	addrGenerator RelayAddressGenerator,
	handler PermissionHandler,
) (*allocation.Manager, error) {
	if handler == nil {
		handler = DefaultPermissionHandler
	}
	if addrGenerator == nil {
		addrGenerator = &nilAddressGenerator{}
	}

	am, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: addrGenerator.AllocatePacketConn,
		AllocateListener:   addrGenerator.AllocateListener,
		AllocateConn:       addrGenerator.AllocateConn,
		PermissionHandler:  handler,
		EventHandler:       s.eventHandler,
		LeveledLogger:      s.log,
	})
	if err != nil {
		return am, err
	}

	s.allocationManagers = append(s.allocationManagers, am)

	return am, err
}

func (s *Server) readLoop(
	conn net.PacketConn,
	allocationManager *allocation.Manager,
	tlsState *tls.ConnectionState,
	dtlsState *dtls.State,
) {
	buf := make([]byte, s.inboundMTU)
	for {
		n, addr, err := conn.ReadFrom(buf)
		switch {
		case err != nil:
			s.log.Debugf("Exit read loop on error: %s", err)

			return
		case n >= s.inboundMTU:
			s.log.Debugf("Read bytes exceeded MTU, packet is possibly truncated")

			continue
		}

		if err := server.HandleRequest(server.Request{
			Conn:                conn,
			SrcAddr:             addr,
			Buff:                buf[:n],
			TLS:                 tlsState,
			DTLS:                dtlsState,
			Log:                 s.log,
			AuthHandler:         s.authHandler,
			QuotaHandler:        s.quotaHandler,
			Realm:               s.realm,
			AllocationManager:   allocationManager,
			ChannelBindTimeout:  s.channelBindTimeout,
			PermissionTimeout:   s.permissionTimeout,
			AllocationLifetime:  s.allocationLifetime,
			StrictAddressFamily: s.strictAddressFamily,
			NonceHash:           s.nonceHash,
		}); err != nil {
			if s.eventHandler.OnAllocationError != nil {
				s.eventHandler.OnAllocationError(addr, conn.LocalAddr(), allocation.UDP.String(), err.Error())
			}
			s.log.Debugf("Failed to handle datagram: %v", err)
		}
	}
}
