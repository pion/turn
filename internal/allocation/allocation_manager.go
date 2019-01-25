package allocation

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pions/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
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

// CreateAllocation creates a new allocation and starts relaying
func (m *Manager) CreateAllocation(fiveTuple *FiveTuple, turnSocket *ipv4.PacketConn, requestedPort int, lifetime uint32) (*Allocation, error) {
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
	}

	listener, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", requestedPort))
	if err != nil {
		return nil, err
	}

	a.RelaySocket = ipv4.NewPacketConn(listener)
	err = a.RelaySocket.SetControlMessage(ipv4.FlagDst, true)
	if err != nil {
		return nil, err
	}

	a.RelayAddr, err = stun.NewTransportAddr(listener.LocalAddr())
	if err != nil {
		return nil, err
	}

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

			allocation.permissionsLock.RLock()
			for _, p := range allocation.permissions {
				p.lifetimeTimer.Stop()
			}
			allocation.permissionsLock.RUnlock()

			allocation.channelBindingsLock.RLock()
			for _, c := range allocation.channelBindings {
				c.lifetimeTimer.Stop()
			}
			allocation.channelBindingsLock.RUnlock()

			m.allocations = append(m.allocations[:i], m.allocations[i+1:]...)
			return true
		}
	}

	return false
}
