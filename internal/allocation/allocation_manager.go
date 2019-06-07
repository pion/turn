package allocation

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/turn/internal/ipnet"
	"github.com/pkg/errors"
)

// Manager is used to hold active allocations
type Manager struct {
	lock        sync.RWMutex
	allocations []*Allocation
}

// GetAllocation fetches the allocation matching the passed FiveTuple
func (m *Manager) GetAllocation(fiveTuple *FiveTuple) *Allocation {
	m.lock.Lock()
	defer m.lock.Unlock()
	for _, a := range m.allocations {
		if a.fiveTuple.Equal(fiveTuple) {
			return a
		}
	}
	return nil
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
func (m *Manager) CreateAllocation(fiveTuple *FiveTuple, turnSocket ipnet.PacketConn, requestedPort int, lifetime uint32) (*Allocation, error) {
	if fiveTuple == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FivTuple")
	}
	if fiveTuple.SrcAddr == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FiveTuple.SrcAddr")
	}
	if fiveTuple.DstAddr == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FiveTuple.DstAddr")
	}
	if a := m.GetAllocation(fiveTuple); a != nil {
		return nil, errors.Errorf("Allocation attempt created with duplicate FiveTuple %v", fiveTuple)
	}
	if turnSocket == nil {
		return nil, errors.Errorf("Allocations must not be created with nil turnSocket")
	}
	if lifetime == 0 {
		return nil, errors.Errorf("Allocations must not be created with a lifetime of 0")
	}

	a := &Allocation{
		fiveTuple:  fiveTuple,
		TurnSocket: turnSocket,
		closed:     make(chan interface{}),
	}

	network := "udp4"
	listener, err := net.ListenPacket(network, fmt.Sprintf("0.0.0.0:%d", requestedPort))
	if err != nil {
		return nil, err
	}

	conn, err := ipnet.NewPacketConn(network, listener)
	if err != nil {
		return nil, err
	}
	a.RelaySocket = conn
	a.RelayAddr = listener.LocalAddr()

	a.lifetimeTimer = time.AfterFunc(time.Duration(lifetime)*time.Second, func() {
		if err := listener.Close(); err != nil {
			fmt.Printf("Failed to close listener for %v \n", a.fiveTuple)
		}
	})

	m.lock.Lock()
	m.allocations = append(m.allocations, a)
	m.lock.Unlock()

	go a.packetHandler(m)
	return a, nil
}

// DeleteAllocation removes an allocation
func (m *Manager) DeleteAllocation(fiveTuple *FiveTuple) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	for i := len(m.allocations) - 1; i >= 0; i-- {
		allocation := m.allocations[i]
		if allocation.fiveTuple.Equal(fiveTuple) {
			if err := allocation.Close(); err != nil {
				fmt.Printf("Failed to close allocation: %v \n", err)
			}
			m.allocations = append(m.allocations[:i], m.allocations[i+1:]...)
			return true
		}
	}

	return false
}
