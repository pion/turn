package allocation

import (
	"net"
	"sync"

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

func CreateAllocation(fiveTuple *FiveTuple, turnSocket *ipv4.PacketConn) (*Allocation, error) {
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

	allocationsLock.Lock()
	allocations = append(allocations, a)
	allocationsLock.Unlock()

	go a.PacketHandler()
	return a, nil
}
