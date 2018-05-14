package allocation

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

var allocationsLock sync.RWMutex
var allocations []*Allocation

// GetAllocation fetches the allocation matching the passed FiveTuple
func GetAllocation(fiveTuple *FiveTuple) *Allocation {
	allocationsLock.Lock()
	defer allocationsLock.Unlock()
	for _, a := range allocations {
		if a.fiveTuple.Equal(fiveTuple) {
			return a
		}
	}
	return nil
}

// CreateAllocation creates a new allocation and starts relaying
func CreateAllocation(fiveTuple *FiveTuple, turnSocket *ipv4.PacketConn, lifetime uint32) (*Allocation, error) {
	if fiveTuple == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FivTuple")
	} else if fiveTuple.SrcAddr == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FiveTuple.SrcAddr")
	} else if fiveTuple.DstAddr == nil {
		return nil, errors.Errorf("Allocations must not be created with nil FiveTuple.DstAddr")
	} else if a := GetAllocation(fiveTuple); a != nil {
		return nil, errors.Errorf("Allocation attempt created with duplicate FiveTuple %v", fiveTuple)
	} else if turnSocket == nil {
		return nil, errors.Errorf("Allocations must not be created with nil turnSocket")
	} else if lifetime == 0 {
		return nil, errors.Errorf("Allocations must not be created with a lifetime of 0")
	}

	a := &Allocation{
		fiveTuple:  fiveTuple,
		TurnSocket: turnSocket,
	}

	listener, err := net.ListenPacket("udp4", "0.0.0.0:0")
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

	allocationsLock.Lock()
	allocations = append(allocations, a)
	allocationsLock.Unlock()

	go a.packetHandler()
	return a, nil
}

func deleteAllocation(fiveTuple *FiveTuple) bool {
	allocationsLock.Lock()
	defer allocationsLock.Unlock()

	for i := len(allocations) - 1; i >= 0; i-- {
		if allocations[i].fiveTuple.Equal(fiveTuple) {

			allocations[i].permissionsLock.RLock()
			for _, p := range allocations[i].permissions {
				p.lifetimeTimer.Stop()
			}
			allocations[i].permissionsLock.RUnlock()

			allocations[i].channelBindingsLock.RLock()
			for _, c := range allocations[i].channelBindings {
				c.lifetimeTimer.Stop()
			}
			allocations[i].channelBindingsLock.RUnlock()

			allocations = append(allocations[:i], allocations[i+1:]...)
			return true
		}
	}

	return false
}
