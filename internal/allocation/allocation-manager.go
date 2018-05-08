package allocation

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pions/pkg/stun"
	"golang.org/x/net/ipv4"
)

var allocationsLock sync.RWMutex
var allocations []*Allocation

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

func CreateAllocation(fiveTuple *FiveTuple, turnSocket *ipv4.PacketConn, lifetime uint32) (*Allocation, error) {
	a := &Allocation{
		fiveTuple:  fiveTuple,
		TurnSocket: turnSocket,
	}

	listener, err := net.ListenPacket("udp", ":0")
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
			fmt.Printf("Failed to close listener for %v", a.fiveTuple)
		}
	})

	allocationsLock.Lock()
	allocations = append(allocations, a)
	allocationsLock.Unlock()

	go a.PacketHandler()
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
