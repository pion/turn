package allocation

import (
	"math/rand"
	"net"
	"testing"

	"github.com/pions/stun"
	"golang.org/x/net/ipv4"
)

var turnSocket *ipv4.PacketConn

func init() {
	c, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	turnSocket = ipv4.NewPacketConn(c)
}

func randomFiveTuple() *FiveTuple {
	return &FiveTuple{
		SrcAddr: &stun.TransportAddr{IP: nil, Port: rand.Int()},
		DstAddr: &stun.TransportAddr{IP: nil, Port: rand.Int()},
	}
}

// test invalid Allocation creations
func TestCreateInvalidAllocation(t *testing.T) {
	if a, err := CreateAllocation(nil, turnSocket, 0, 5000); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil FiveTuple")
	}
	if a, err := CreateAllocation(randomFiveTuple(), nil, 0, 5000); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil turnSocket")
	}
	if a, err := CreateAllocation(randomFiveTuple(), turnSocket, 0, 0); a != nil || err == nil {
		t.Errorf("Illegally created allocation with 0 lifetime")
	}
}

// test valid Allocation creations
func TestCreateAllocation(t *testing.T) {
	fiveTuple := randomFiveTuple()
	if a, err := CreateAllocation(fiveTuple, turnSocket, 0, 5000); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a := GetAllocation(fiveTuple); a == nil {
		t.Errorf("Failed to get allocation right after creation")
	}
}

// test that two allocations can't be created with the same FiveTuple
func TestCreateAllocationDuplicateFiveTuple(t *testing.T) {
	fiveTuple := randomFiveTuple()
	if a, err := CreateAllocation(fiveTuple, turnSocket, 0, 5000); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a, err := CreateAllocation(fiveTuple, turnSocket, 0, 5000); a != nil || err == nil {
		t.Errorf("Was able to create allocation with same FiveTuple twice")
	}
}
