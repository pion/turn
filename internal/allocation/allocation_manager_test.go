package allocation

import (
	"math/rand"
	"net"
	"testing"

	"github.com/pions/stun"
	"golang.org/x/net/ipv4"
)

func TestAllocation(t *testing.T) {
	tt := []struct {
		name string
		f    func(*testing.T, *ipv4.PacketConn)
	}{
		{"CreateInvalidAllocation", subTestCreateInvalidAllocation},
		{"CreateAllocation", subTestCreateAllocation},
		{"CreateAllocationDuplicateFiveTuple", subTestCreateAllocationDuplicateFiveTuple},
	}

	c, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	turnSocket := ipv4.NewPacketConn(c)

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t, turnSocket)
		})
	}
}

// test invalid Allocation creations
func subTestCreateInvalidAllocation(t *testing.T, turnSocket *ipv4.PacketConn) {
	m := &Manager{}
	if a, err := m.CreateAllocation(nil, turnSocket, 0, 5000); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil FiveTuple")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), nil, 0, 5000); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil turnSocket")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, 0); a != nil || err == nil {
		t.Errorf("Illegally created allocation with 0 lifetime")
	}
}

// test valid Allocation creations
func subTestCreateAllocation(t *testing.T, turnSocket *ipv4.PacketConn) {
	m := &Manager{}
	fiveTuple := randomFiveTuple()
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, 5000); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a := m.GetAllocation(fiveTuple); a == nil {
		t.Errorf("Failed to get allocation right after creation")
	}
}

// test that two allocations can't be created with the same FiveTuple
func subTestCreateAllocationDuplicateFiveTuple(t *testing.T, turnSocket *ipv4.PacketConn) {
	m := &Manager{}
	fiveTuple := randomFiveTuple()
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, 5000); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, 5000); a != nil || err == nil {
		t.Errorf("Was able to create allocation with same FiveTuple twice")
	}
}

func randomFiveTuple() *FiveTuple {
	/* #nosec */
	return &FiveTuple{
		SrcAddr: &stun.TransportAddr{IP: nil, Port: rand.Int()},
		DstAddr: &stun.TransportAddr{IP: nil, Port: rand.Int()},
	}
}
