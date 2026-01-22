// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddr_FromUDPAddr(t *testing.T) {
	u := &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 1234,
	}
	a := new(Addr)
	a.FromUDPAddr(u)
	assert.True(t, u.IP.Equal(a.IP))
	assert.Equal(t, u.Port, a.Port)
	assert.Equal(t, u.String(), a.String())
	assert.Equal(t, "turn", a.Network())
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
	assert.False(t, a.Equal(b))
	assert.True(t, a.EqualIP(b))
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
		assert.Equal(t, tc.v, tc.a.Equal(tc.b), "(%s) %s [%v!=%v] %s", tc.name, tc.a, tc.v, tc.b, tc.b)
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
	assert.Equal(t, "127.0.0.1:200->127.0.0.1:100 (UDP)", s, "unexpected stringer output")
}
