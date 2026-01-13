// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/logging"
	"github.com/pion/randutil"
	"github.com/pion/turn/v4/internal/proto"
)

// If no ConnectionBind request associated with this peer data
// connection is received after 30 seconds, the peer data connection
// MUST be closed.
const defaultTCPConnectionBindTimeout = time.Second * 30

// ManagerConfig a bag of config params for Manager.
type ManagerConfig struct {
	LeveledLogger      logging.LeveledLogger
	AllocatePacketConn func(network string, requestedPort int) (net.PacketConn, net.Addr, error)
	AllocateListener   func(network string, requestedPort int) (net.Listener, net.Addr, error)
	AllocateConn       func(network string, laddr, raddr net.Addr) (net.Conn, error)
	PermissionHandler  func(sourceAddr net.Addr, peerIP net.IP) bool
	EventHandler       EventHandler

	tcpConnectionBindTimeout time.Duration
}

type reservation struct {
	token string
	port  int
}

// Manager is used to hold active allocations.
type Manager struct {
	lock                     sync.RWMutex
	log                      logging.LeveledLogger
	tcpConnectionBindTimeout time.Duration

	allocations  map[FiveTupleFingerprint]*Allocation
	reservations []*reservation

	allocatePacketConn func(network string, requestedPort int) (net.PacketConn, net.Addr, error)
	allocateListener   func(network string, requestedPort int) (net.Listener, net.Addr, error)
	allocateConn       func(network string, laddr, raddr net.Addr) (net.Conn, error)
	permissionHandler  func(sourceAddr net.Addr, peerIP net.IP) bool
	EventHandler       EventHandler
}

// NewManager creates a new instance of Manager.
func NewManager(config ManagerConfig) (*Manager, error) {
	switch {
	case config.AllocatePacketConn == nil:
		return nil, errAllocatePacketConnMustBeSet
	case config.AllocateListener == nil:
		return nil, errAllocateListenerMustBeSet
	case config.AllocateConn == nil:
		return nil, errAllocateConnMustBeSet
	case config.LeveledLogger == nil:
		return nil, errLeveledLoggerMustBeSet
	}

	tcpConnectionBindTimeout := config.tcpConnectionBindTimeout
	if tcpConnectionBindTimeout == 0 {
		tcpConnectionBindTimeout = defaultTCPConnectionBindTimeout
	}

	return &Manager{
		log:                      config.LeveledLogger,
		allocations:              make(map[FiveTupleFingerprint]*Allocation, 64),
		allocatePacketConn:       config.AllocatePacketConn,
		allocateListener:         config.AllocateListener,
		allocateConn:             config.AllocateConn,
		permissionHandler:        config.PermissionHandler,
		EventHandler:             config.EventHandler,
		tcpConnectionBindTimeout: tcpConnectionBindTimeout,
	}, nil
}

// GetAllocation fetches the allocation matching the passed FiveTuple.
func (m *Manager) GetAllocation(fiveTuple *FiveTuple) *Allocation {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.allocations[fiveTuple.Fingerprint()]
}

// GetAllocationForUsername fetches the allocation matching the passed FiveTuple and Username.
func (m *Manager) GetAllocationForUsername(fiveTuple *FiveTuple, username string) *Allocation {
	allocation := m.GetAllocation(fiveTuple)
	if allocation != nil && allocation.username == username {
		return allocation
	}

	return nil
}

// AllocationCount returns the number of existing allocations.
func (m *Manager) AllocationCount() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return len(m.allocations)
}

// Close closes the manager and closes all allocations it manages.
func (m *Manager) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, a := range m.allocations {
		if err := a.Close(); err != nil {
			return err
		}
	}

	return nil
}

// CreateAllocation creates a new allocation and starts relaying.
func (m *Manager) CreateAllocation( // nolint: cyclop
	fiveTuple *FiveTuple,
	turnSocket net.PacketConn,
	protocol proto.Protocol,
	requestedPort int,
	lifetime time.Duration,
	username, realm string,
	addressFamily proto.RequestedAddressFamily,
) (*Allocation, error) {
	switch {
	case fiveTuple == nil:
		return nil, errNilFiveTuple
	case fiveTuple.SrcAddr == nil:
		return nil, errNilFiveTupleSrcAddr
	case fiveTuple.DstAddr == nil:
		return nil, errNilFiveTupleDstAddr
	case turnSocket == nil:
		return nil, errNilTurnSocket
	case lifetime == 0:
		return nil, errLifetimeZero
	}

	if alloc := m.GetAllocation(fiveTuple); alloc != nil {
		return nil, fmt.Errorf("%w: %v", errDupeFiveTuple, fiveTuple)
	}
	alloc := NewAllocation(turnSocket, fiveTuple, m.EventHandler, m.log)
	alloc.username = username
	alloc.realm = realm
	alloc.addressFamily = addressFamily

	switch protocol {
	case proto.ProtoUDP:
		network := "udp4"
		if addressFamily == proto.RequestedFamilyIPv6 {
			network = "udp6"
		}
		conn, relayAddr, err := m.allocatePacketConn(network, requestedPort)
		if err != nil {
			return nil, err
		}
		alloc.relayPacketConn = conn
		alloc.RelayAddr = relayAddr
	case proto.ProtoTCP:
		network := "tcp4"
		if addressFamily == proto.RequestedFamilyIPv6 {
			network = "tcp6"
		}
		ln, relayAddr, err := m.allocateListener(network, requestedPort)
		if err != nil {
			return nil, err
		}
		alloc.relayListener = ln
		alloc.RelayAddr = relayAddr
	}

	m.log.Debugf("Listening on relay address: %s", alloc.RelayAddr)

	alloc.lifetimeTimer = time.AfterFunc(lifetime, func() {
		m.DeleteAllocation(alloc.fiveTuple)
	})

	m.lock.Lock()
	m.allocations[fiveTuple.Fingerprint()] = alloc
	m.lock.Unlock()

	if m.EventHandler.OnAllocationCreated != nil {
		m.EventHandler.OnAllocationCreated(fiveTuple.SrcAddr, fiveTuple.DstAddr,
			fiveTuple.Protocol.String(), username, realm, alloc.RelayAddr, requestedPort)
	}

	// Only start the UDP relay loop for UDP allocations.
	if alloc.relayPacketConn != nil {
		go alloc.packetConnHandler(m)
	}
	// For TCP allocations, accept inbound connections on the relayed listener and notify the client.
	if alloc.relayListener != nil {
		go alloc.connHandler(m)
	}

	return alloc, nil
}

// DeleteAllocation removes an allocation.
func (m *Manager) DeleteAllocation(fiveTuple *FiveTuple) {
	fingerprint := fiveTuple.Fingerprint()

	m.lock.Lock()
	allocation := m.allocations[fingerprint]
	delete(m.allocations, fingerprint)
	m.lock.Unlock()

	if allocation == nil {
		return
	}

	m.lock.Lock()
	if err := allocation.Close(); err != nil {
		m.log.Errorf("Failed to close allocation: %v", err)
	}
	m.lock.Unlock()

	if m.EventHandler.OnAllocationDeleted != nil {
		m.EventHandler.OnAllocationDeleted(fiveTuple.SrcAddr, fiveTuple.DstAddr,
			fiveTuple.Protocol.String(), allocation.username, allocation.realm)
	}
}

// CreateReservation stores the reservation for the token+port.
func (m *Manager) CreateReservation(reservationToken string, port int) {
	time.AfterFunc(30*time.Second, func() {
		m.lock.Lock()
		defer m.lock.Unlock()
		for i := len(m.reservations) - 1; i >= 0; i-- {
			if m.reservations[i].token == reservationToken {
				m.reservations = append(m.reservations[:i], m.reservations[i+1:]...)

				return
			}
		}
	})

	m.lock.Lock()
	m.reservations = append(m.reservations, &reservation{
		token: reservationToken,
		port:  port,
	})
	m.lock.Unlock()
}

// GetReservation returns the port for a given reservation if it exists.
func (m *Manager) GetReservation(reservationToken string) (int, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	for _, r := range m.reservations {
		if r.token == reservationToken {
			return r.port, true
		}
	}

	return 0, false
}

// GetRandomEvenPort returns a random un-allocated udp4 port.
func (m *Manager) GetRandomEvenPort() (int, error) {
	for i := 0; i < 128; i++ {
		conn, addr, err := m.allocatePacketConn("udp4", 0)
		if err != nil {
			return 0, err
		}
		udpAddr, ok := addr.(*net.UDPAddr)
		err = conn.Close()
		if err != nil {
			return 0, err
		}

		if !ok {
			return 0, errFailedToCastUDPAddr
		}
		if udpAddr.Port%2 == 0 {
			return udpAddr.Port, nil
		}
	}

	return 0, errFailedToAllocateEvenPort
}

// GrantPermission handles permission requests by calling the permission handler callback
// associated with the TURN server listener socket.
func (m *Manager) GrantPermission(sourceAddr net.Addr, peerIP net.IP) error {
	// No permission handler: open
	if m.permissionHandler == nil {
		return nil
	}

	if m.permissionHandler(sourceAddr, peerIP) {
		return nil
	}

	return errAdminProhibited
}

// CreateTCPConnection creates a new outbound TCP Connection and returns the Connection-ID
// if it succeeds.
func (m *Manager) CreateTCPConnection( // nolint: cyclop
	allocation *Allocation,
	peerAddress proto.PeerAddress,
) (proto.ConnectionID, error) {
	if len(peerAddress.IP) == 0 || peerAddress.Port == 0 {
		return 0, errInvalidPeerAddress
	}

	relayAddr := allocation.RelayAddr
	if allocation.RelayAddr == nil {
		m.log.Warn("Failed to create TCP Connection: Relay address not available")

		return 0, ErrTCPConnectionTimeoutOrFailure
	}

	remoteAddr := &net.TCPAddr{IP: peerAddress.IP, Port: peerAddress.Port}

	m.lock.Lock()
	if m.isDupeTCPConnection(allocation, remoteAddr) {
		return 0, ErrDupeTCPConnection
	}
	m.lock.Unlock()

	conn, err := m.allocateConn("tcp4", relayAddr, remoteAddr) // nolint: noctx
	if err != nil {
		m.log.Warnf("Failed to create TCP Connection: %v", err)

		return 0, ErrTCPConnectionTimeoutOrFailure
	}

	connectionID, err := m.addTCPConnection(allocation, conn)
	if err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			m.log.Warnf("Failed to close TCP connection after ConnectionID generation failed: %v", closeErr)
		}
	}

	return connectionID, err
}

func (m *Manager) addTCPConnection(allocation *Allocation, conn net.Conn) (proto.ConnectionID, error) {
	rand64, err := randutil.CryptoUint64()
	if err != nil {
		return 0, err
	}

	connectionID := proto.ConnectionID(uint32(rand64 >> 32)) // nolint: gosec

	m.lock.Lock()
	defer m.lock.Unlock()

	for _, a := range m.allocations {
		if _, ok := a.tcpConnections[connectionID]; ok {
			return 0, errFailedToGenerateConnectionID
		}
	}

	newConnAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return 0, ErrDupeTCPConnection
	}

	if m.isDupeTCPConnection(allocation, newConnAddr) {
		return 0, ErrDupeTCPConnection
	}

	tcpConn := &tcpConnection{conn, atomic.Bool{}, nil}
	allocation.tcpConnections[connectionID] = tcpConn
	tcpConn.bindTimer = time.AfterFunc(m.tcpConnectionBindTimeout, func() {
		if !tcpConn.isBound.Load() {
			m.log.Warnf("Removing TCP Connection that was never bound %v %v", connectionID, allocation.fiveTuple)
			allocation.RemoveTCPConnection(m, connectionID)
		}
	})

	return connectionID, nil
}

func (m *Manager) isDupeTCPConnection(allocation *Allocation, remoteAddr *net.TCPAddr) bool {
	for i := range allocation.tcpConnections {
		tcpAddr, ok := allocation.tcpConnections[i].RemoteAddr().(*net.TCPAddr)
		if !ok {
			return true
		} else if tcpAddr.IP.Equal(remoteAddr.IP) && tcpAddr.Port == remoteAddr.Port {
			return true
		}
	}

	return false
}

// GetTCPConnection returns the TCP Connection for the given ConnectionID.
func (m *Manager) GetTCPConnection(username string, connectionID proto.ConnectionID) net.Conn {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, a := range m.allocations {
		if tcpConnection, ok := a.tcpConnections[connectionID]; ok {
			if a.username != username || tcpConnection.isBound.Swap(true) {
				return nil
			}

			tcpConnection.bindTimer.Stop()

			return tcpConnection
		}
	}

	return nil
}

func (m *Manager) RemoveTCPConnection(connectionID proto.ConnectionID) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, a := range m.allocations {
		if _, ok := a.tcpConnections[connectionID]; ok {
			a.removeTCPConnection(connectionID)
		}
	}
}
