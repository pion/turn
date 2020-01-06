package proto

import (
	"fmt"
	"net"
	"testing"
)

func TestAddr_FromUDPAddr(t *testing.T) {
	u := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 1234,
	}
	a := new(Addr)
	a.FromUDPAddr(u)
	if !u.IP.Equal(a.IP) || u.Port != a.Port || u.String() != a.String() {
		t.Error("not equal")
	}
	if a.Network() != "turn" {
		t.Error("unexpected network")
	}
}

func TestAddr_EqualIP(t *testing.T) {
	a := Addr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 1337,
	}
	b := Addr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 1338,
	}
	if a.Equal(b) {
		t.Error("a != b")
	}
	if !a.EqualIP(b) {
		t.Error("a.IP should equal to b.IP")
	}
}

func TestFiveTuple_Equal(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b FiveTuple
		v    bool
	}{
		{
			name: "blank",
			v:    true,
		},
		{
			name: "proto",
			a: FiveTuple{
				Proto: ProtoUDP,
			},
		},
		{
			name: "server",
			a: FiveTuple{
				Server: Addr{
					Port: 100,
				},
			},
		},
		{
			name: "client",
			a: FiveTuple{
				Client: Addr{
					Port: 100,
				},
			},
		},
	} {
		if v := tc.a.Equal(tc.b); v != tc.v {
			t.Errorf("(%s) %s [%v!=%v] %s",
				tc.name, tc.a, v, tc.v, tc.b,
			)
		}
	}
}

func TestFiveTuple_String(t *testing.T) {
	s := fmt.Sprint(FiveTuple{
		Proto: ProtoUDP,
		Server: Addr{
			Port: 100,
			IP:   net.IPv4(127, 0, 0, 1),
		},
		Client: Addr{
			Port: 200,
			IP:   net.IPv4(127, 0, 0, 1),
		},
	})
	if s != "127.0.0.1:200->127.0.0.1:100 (UDP)" {
		t.Error("unexpected stringer output")
	}
}
