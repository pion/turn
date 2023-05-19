// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2/internal/proto"
)

// ManagerConfig a bag of config params for Manager.
type ManagerConfig struct {
	LeveledLogger      logging.LeveledLogger
	AllocatePacketConn func(network string, requestedPort int) (net.PacketConn, net.Addr, error)
	AllocateConn       func(network string, requestedPort int) (net.Conn, net.Addr, error)
	AllocateListener   func(network string, requestedPort int) (net.Listener, net.Addr, error)
	PermissionHandler  func(sourceAddr net.Addr, peerIP net.IP) bool
	Protocol           Protocol
}

type reservation struct {
	token string
	port  int
}

// Manager is used to hold active allocations
type Manager struct {
	lock sync.RWMutex
	log  logging.LeveledLogger

	allocations  map[string]*Allocation
	reservations []*reservation

	allocatePacketConn func(network string, requestedPort int) (net.PacketConn, net.Addr, error)
	allocateConn       func(network string, requestedPort int) (net.Conn, net.Addr, error)
	allocateListener   func(network string, requestedPort int) (net.Listener, net.Addr, error)
	permissionHandler  func(sourceAddr net.Addr, peerIP net.IP) bool

	waitingConns map[proto.ConnectionID]*Allocation
	runningConns map[proto.ConnectionID]*Allocation
	Protocol     Protocol
}

// NewManager creates a new instance of Manager.
func NewManager(config ManagerConfig) (*Manager, error) {
	switch {
	case config.AllocatePacketConn == nil:
		return nil, errAllocatePacketConnMustBeSet
	case config.AllocateListener == nil:
		return nil, errAllocateListenerMustBeSet
	case config.LeveledLogger == nil:
		return nil, errLeveledLoggerMustBeSet
	}

	return &Manager{
		log:                config.LeveledLogger,
		allocations:        make(map[string]*Allocation, 64),
		allocatePacketConn: config.AllocatePacketConn,
		allocateConn:       config.AllocateConn,
		allocateListener:   config.AllocateListener,
		permissionHandler:  config.PermissionHandler,
		waitingConns:       make(map[proto.ConnectionID]*Allocation),
		runningConns:       make(map[proto.ConnectionID]*Allocation),
		Protocol:           config.Protocol,
	}, nil
}

// GetAllocation fetches the allocation matching the passed FiveTuple
func (m *Manager) GetAllocation(fiveTuple *FiveTuple) *Allocation {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.allocations[fiveTuple.Fingerprint()]
}

// AllocationCount returns the number of existing allocations
func (m *Manager) AllocationCount() int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return len(m.allocations)
}

// Close closes the manager and closes all allocations it manages
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

// CreateAllocation creates a new allocation and starts relaying
func (m *Manager) CreateAllocation(fiveTuple *FiveTuple, turnSocket net.PacketConn, requestedPort int, lifetime time.Duration, requestedTransportProtocol proto.Protocol) (*Allocation, error) {
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

	if a := m.GetAllocation(fiveTuple); a != nil {
		return nil, fmt.Errorf("%w: %v", errDupeFiveTuple, fiveTuple)
	}
	a := NewAllocation(turnSocket, fiveTuple, m.log, requestedTransportProtocol)

	if a.RequestedTransportProtocol == proto.ProtoTCP {
		listener, relayAddr, err := m.allocateListener("tcp", requestedPort)
		if err != nil {
			return nil, err
		}
		a.RelayListener = listener
		a.RelayAddr = relayAddr
	} else {
		conn, relayAddr, err := m.allocatePacketConn("udp4", requestedPort)
		if err != nil {
			return nil, err
		}

		a.RelaySocket = conn
		a.RelayAddr = relayAddr
	}

	m.log.Debugf("listening on relay addr: %s", a.RelayAddr.String())

	a.lifetimeTimer = time.AfterFunc(lifetime, func() {
		m.DeleteAllocation(a.fiveTuple)
	})

	m.lock.Lock()
	m.allocations[fiveTuple.Fingerprint()] = a
	m.lock.Unlock()

	if a.RequestedTransportProtocol == proto.ProtoTCP {
		go a.connectionHandler(m)
	} else {
		go a.packetHandler(m)
	}
	return a, nil
}

// DeleteAllocation removes an allocation
func (m *Manager) DeleteAllocation(fiveTuple *FiveTuple) {
	fingerprint := fiveTuple.Fingerprint()

	m.lock.Lock()
	allocation := m.allocations[fingerprint]
	delete(m.allocations, fingerprint)
	m.lock.Unlock()

	if allocation == nil {
		return
	}

	if err := allocation.Close(); err != nil {
		m.log.Errorf("Failed to close allocation: %v", err)
	}
}

// CreateReservation stores the reservation for the token+port
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

// GetReservation returns the port for a given reservation if it exists
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

// GetRandomEvenPort returns a random un-allocated udp4 port
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
// associated with the TURN server listener socket
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

func (m *Manager) BindConnection(cid proto.ConnectionID) net.Conn {
	m.lock.Lock()
	defer m.lock.Unlock()
	a := m.waitingConns[cid]
	delete(m.waitingConns, cid)
	if a == nil {
		return nil
	}
	m.runningConns[cid] = a
	return a.GetConnectionByID(cid)
}

func (m *Manager) DeleteConnection(cid proto.ConnectionID) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.runningConns, cid)
}

func (m *Manager) Connect(a *Allocation, dst net.Addr) (proto.ConnectionID, error) {
	cid := m.newCID(a)

	err := a.newConnection(cid, dst.String())
	if err != nil {
		return 0, err
	}

	// If no ConnectionBind request associated with this peer data
	// connection is received after 30 seconds, the peer data connection
	// MUST be closed.
	go m.removeAfter30(cid, dst)

	return cid, nil
}

func (m *Manager) removeAfter30(cid proto.ConnectionID, dst net.Addr) {
	<-time.After(30 * time.Second)
	m.lock.Lock()
	defer m.lock.Unlock()
	a, ok := m.waitingConns[cid]
	if !ok {
		return
	}
	delete(m.waitingConns, cid)
	a.removeConnection(cid, dst.String())
}

func (m *Manager) newCID(a *Allocation) proto.ConnectionID {
	m.lock.Lock()
	var cid proto.ConnectionID
	for {
		cid = proto.ConnectionID(rand.Uint32())
		if cid == 0 {
			continue
		} else if _, ok := m.waitingConns[cid]; ok {
			continue
		} else if _, ok := m.runningConns[cid]; ok {
			continue
		} else {
			break
		}
	}
	m.waitingConns[cid] = a
	m.lock.Unlock()

	return cid
}
