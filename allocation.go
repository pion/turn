// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package turn contains the public API for pion/turn, a toolkit for building TURN clients and servers.
package turn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/deadline"
	"github.com/pion/transport/v4/packetio"
	"github.com/pion/turn/v4/internal/client"
)

const defaultAcceptBacklog = 128

var (
	_ transport.Dialer = &turnAllocation{}
	_ net.Listener     = &turnAllocation{}
	_ Allocation       = &turnAllocation{}
)

// Typed errors.
var (
	ErrAllocationClosed = errors.New("allocation: closed")
	errFailedToAlloc    = errors.New("allocation: failed to create allocation")
	errFailedToDial     = errors.New("allocation: failed to dial")
	errInvalidNetwork   = errors.New("allocation: invalid network")
)

// Allocation represents a TURN allocation that provides bidirectional communication with peers
// through a TURN relay server. It combines dialer and listener semantics: use Dial to establish
// outgoing connections to peers, and Accept to receive incoming connections from peers. The
// allocation is bound to a specific transport protocol (UDP or TCP) specified when created via
// NewAllocation. All connections through this allocation use that protocol.
type Allocation interface {
	// Dial establishes a connection to the specified peer address through the TURN relay.
	// The network parameter must match the allocation's protocol ("udp" or "tcp").
	// Dial automatically creates a TURN permission for the peer, allowing the peer to
	// send data back to this allocation's relay address.
	Dial(network, address string) (net.Conn, error)

	// Accept waits for the next incoming connection from a peer.  The peer must have a valid
	// permission (created via Dial or CreatePermission) to send data to this allocation's
	// relay address.
	Accept() (net.Conn, error)

	// Addr returns the relay transport address of the allocation.
	Addr() net.Addr

	// Close releases the TURN allocation and closes all associated connections.
	// Any blocked Accept or Dial calls will return ErrAllocationClosed.
	Close() error

	// CreatePermission installs a TURN permission for the specified peer address,
	// allowing that peer to send data to this allocation's relay address.
	CreatePermission(addr net.Addr) error
}

type turnAllocation struct {
	*Client    // TURN client
	network    string
	tcpAlloc   *client.TCPAllocation // for TCP allocations
	dispatcher *packetDispatcher     // for UDP allocations
	closeCh    chan struct{}
	closeOnce  sync.Once
	mu         sync.RWMutex
	closed     bool
}

// NewAllocation creates a TURN allocation that can dial peers and accept incoming connections.
func NewAllocation(network string, conf *ClientConfig) (Allocation, error) {
	turnClient, err := NewClient(conf)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToAlloc, err)
	}

	if err = turnClient.Listen(); err != nil {
		turnClient.Close()

		return nil, fmt.Errorf("%w: %w", errFailedToAlloc, err)
	}

	turnAlloc := &turnAllocation{
		Client:  turnClient,
		network: network,
		closeCh: make(chan struct{}),
	}

	switch network {
	case "udp", "udp4", "udp6": //nolint:goconst
		pconn, err := turnClient.Allocate()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errFailedToAlloc, err)
		}
		turnAlloc.dispatcher = newPacketDispatcher(pconn, turnAlloc.closeCh)
	case "tcp", "tcp4", "tcp6": //nolint:goconst
		tcpAlloc, err := turnClient.AllocateTCP()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errFailedToAlloc, err)
		}
		turnAlloc.tcpAlloc = tcpAlloc
	default:
		return nil, errInvalidNetwork
	}

	return turnAlloc, nil
}

// Dial establishes a connection to the address through TURN.  Network must be "udp", "udp4",
// "udp6" or "tcp", "tcp4", "tcp6".
func (a *turnAllocation) Dial(network, address string) (net.Conn, error) {
	a.mu.RLock()
	if a.closed {
		a.mu.RUnlock()

		return nil, ErrAllocationClosed
	}
	a.mu.RUnlock()

	switch a.network {
	case "udp", "udp4", "udp6": //nolint:goconst
		return a.dialUDP(network, address)
	case "tcp", "tcp4", "tcp6": //nolint:goconst
		return a.dialTCP(network, address)
	default:
		return nil, errInvalidNetwork
	}
}

func (a *turnAllocation) dialUDP(network, address string) (net.Conn, error) {
	if network != a.network {
		return nil, errInvalidNetwork
	}

	peerAddr, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToDial, err)
	}

	if err = a.CreatePermission(peerAddr); err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToDial, err)
	}

	return a.dispatcher.register(peerAddr), nil
}

func (a *turnAllocation) dialTCP(network, address string) (net.Conn, error) {
	if network != a.network {
		return nil, errInvalidNetwork
	}

	peerAddr, err := net.ResolveTCPAddr(network, address)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToDial, err)
	}

	if err = a.CreatePermission(peerAddr); err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToDial, err)
	}

	return a.tcpAlloc.DialTCP(network, nil, peerAddr)
}

// Accept returns the next incoming connection from a permitted peer.
func (a *turnAllocation) Accept() (net.Conn, error) {
	switch a.network {
	case "udp", "udp4", "udp6": //nolint:goconst
		select {
		case <-a.closeCh:
			return nil, ErrAllocationClosed
		case conn := <-a.dispatcher.acceptCh:
			return conn, nil
		}
	case "tcp", "tcp4", "tcp6": //nolint:goconst
		return a.tcpAlloc.Accept()
	default:
		return nil, errInvalidNetwork
	}
}

// Addr returns the transport relay address of the allocation.
func (a *turnAllocation) Addr() net.Addr {
	switch a.network {
	case "udp", "udp4", "udp6": //nolint:goconst
		return a.dispatcher.pconn.LocalAddr()
	case "tcp", "tcp4", "tcp6": //nolint:goconst
		return a.tcpAlloc.RelayAddr()
	default:
		return nil
	}
}

// Close closes the allocation and all associated connections.
func (a *turnAllocation) Close() error {
	var err error
	a.closeOnce.Do(func() {
		switch a.network {
		case "udp", "udp4", "udp6": //nolint:goconst
			err = a.dispatcher.pconn.Close()
		case "tcp", "tcp4", "tcp6": //nolint:goconst
			err = a.tcpAlloc.Close()
		default:
			err = errInvalidNetwork
		}
		a.Client.Close()
		a.mu.Lock()
		a.closed = true
		a.mu.Unlock()
		close(a.closeCh)
	})

	return err
}

// CreatePermission admits a peer on the allocation. Note that Dial automatically creates a
// permission for the peer.
func (a *turnAllocation) CreatePermission(addr net.Addr) error {
	return a.Client.CreatePermission(addr)
}

const receiveMTU = 8192

// packetDispatcher demultiplexes packets from a single PacketConn to multiple Conns.
type packetDispatcher struct {
	pconn    net.PacketConn
	mu       sync.Mutex
	conns    map[string]*dispatchedConn
	acceptCh chan *dispatchedConn
	closeCh  chan struct{}
}

// newPacketDispatcher creates a dispatcher and starts its read loop.
func newPacketDispatcher(pconn net.PacketConn, closeCh chan struct{}) *packetDispatcher {
	d := &packetDispatcher{
		pconn:    pconn,
		conns:    make(map[string]*dispatchedConn),
		acceptCh: make(chan *dispatchedConn, defaultAcceptBacklog),
		closeCh:  closeCh,
	}

	go d.readLoop()

	return d
}

func (d *packetDispatcher) readLoop() {
	buf := make([]byte, receiveMTU)
	for {
		n, raddr, err := d.pconn.ReadFrom(buf)
		if err != nil {
			// Close all conn buffers on read error.
			d.mu.Lock()
			for _, c := range d.conns {
				_ = c.buffer.Close()
			}
			d.mu.Unlock()

			return
		}
		d.dispatch(raddr, buf[:n])
	}
}

func (d *packetDispatcher) dispatch(raddr net.Addr, buf []byte) {
	d.mu.Lock()
	conn, ok := d.conns[raddr.String()]
	if !ok {
		// New peer - create conn and send to accept channel.
		conn = d.newConn(raddr)
		d.conns[raddr.String()] = conn
		select {
		case d.acceptCh <- conn:
			// Sent to accept queue.
		default:
			// Accept queue full: unreg and drop connection
			delete(d.conns, raddr.String())
		}
	}
	d.mu.Unlock()
	_, _ = conn.buffer.Write(buf)
}

// register adds a conn for the given peer address (used by Dial).
func (d *packetDispatcher) register(raddr net.Addr) *dispatchedConn {
	d.mu.Lock()
	defer d.mu.Unlock()

	if conn, ok := d.conns[raddr.String()]; ok {
		return conn
	}

	conn := d.newConn(raddr)
	d.conns[raddr.String()] = conn

	return conn
}

// newConn creates a new dispatchedConn (must be called with lock held).
func (d *packetDispatcher) newConn(raddr net.Addr) *dispatchedConn {
	return &dispatchedConn{
		dispatcher:    d,
		rAddr:         raddr,
		buffer:        packetio.NewBuffer(),
		writeDeadline: deadline.New(),
	}
}

// unregister removes a conn from the dispatcher.
func (d *packetDispatcher) unregister(raddr net.Addr) {
	d.mu.Lock()
	delete(d.conns, raddr.String())
	d.mu.Unlock()
}

// dispatchedConn is a net.Conn bound to a specific peer, receiving via the dispatcher.
type dispatchedConn struct {
	dispatcher    *packetDispatcher
	rAddr         net.Addr
	buffer        *packetio.Buffer
	writeDeadline *deadline.Deadline
	closeOnce     sync.Once
}

// Read reads data from the buffer (dispatched by the read loop).
func (c *dispatchedConn) Read(b []byte) (int, error) {
	return c.buffer.Read(b)
}

// Write sends data to the bound peer.
func (c *dispatchedConn) Write(b []byte) (int, error) {
	select {
	case <-c.writeDeadline.Done():
		return 0, context.DeadlineExceeded
	default:
	}

	return c.dispatcher.pconn.WriteTo(b, c.rAddr)
}

// Close unregisters from dispatcher and closes the buffer.
func (c *dispatchedConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.dispatcher.unregister(c.rAddr)
		err = c.buffer.Close()
	})

	return err
}

// LocalAddr returns the local address (relay address).
func (c *dispatchedConn) LocalAddr() net.Addr {
	return c.dispatcher.pconn.LocalAddr()
}

// RemoteAddr returns the bound peer address.
func (c *dispatchedConn) RemoteAddr() net.Addr {
	return c.rAddr
}

// SetDeadline sets both read and write deadlines.
func (c *dispatchedConn) SetDeadline(t time.Time) error {
	c.writeDeadline.Set(t)

	return c.buffer.SetReadDeadline(t)
}

// SetReadDeadline sets the read deadline.
func (c *dispatchedConn) SetReadDeadline(t time.Time) error {
	return c.buffer.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *dispatchedConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Set(t)

	return nil
}
