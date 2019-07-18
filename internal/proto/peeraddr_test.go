package proto

import (
	"net"
	"testing"

	"github.com/pion/stun"
)

func TestPeerAddress(t *testing.T) {
	// Simple tests because already tested in stun.
	a := PeerAddress{
		IP:   net.IPv4(111, 11, 1, 2),
		Port: 333,
	}
	t.Run("String", func(t *testing.T) {
		if a.String() != "111.11.1.2:333" {
			t.Error("invalid string")
		}
	})
	m := new(stun.Message)
	if err := a.AddTo(m); err != nil {
		t.Fatal(err)
	}
	m.WriteHeader()
	decoded := new(stun.Message)
	decoded.Write(m.Raw)
	var aGot PeerAddress
	if err := aGot.GetFrom(decoded); err != nil {
		t.Fatal(err)
	}
}
