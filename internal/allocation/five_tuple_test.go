package allocation

import (
	"net"
	"testing"
)

func TestFiveTupleProtocol(t *testing.T) {
	udpExpect := Protocol(0)
	tcpExpect := Protocol(1)

	if udpExpect != UDP {
		t.Errorf("Invalid UDP Protocol value, expect %d but %d", udpExpect, UDP)
	}

	if tcpExpect != TCP {
		t.Errorf("Invalid TCP Protocol value, expect %d but %d", tcpExpect, TCP)
	}
}

func TestFiveTupleEqual(t *testing.T) {
	srcAddr1, _ := net.ResolveUDPAddr("udp", "0.0.0.0:3478")
	srcAddr2, _ := net.ResolveUDPAddr("udp", "0.0.0.0:3479")

	dstAddr1, _ := net.ResolveUDPAddr("udp", "0.0.0.0:3480")
	dstAddr2, _ := net.ResolveUDPAddr("udp", "0.0.0.0:3481")

	tt := []struct {
		name   string
		expect bool
		a      *FiveTuple
		b      *FiveTuple
	}{
		{
			"Equal",
			true,
			&FiveTuple{UDP, srcAddr1, dstAddr1},
			&FiveTuple{UDP, srcAddr1, dstAddr1},
		},
		{
			"DifferentProtocol",
			false,
			&FiveTuple{UDP, srcAddr1, dstAddr1},
			&FiveTuple{TCP, srcAddr1, dstAddr1},
		},
		{
			"DifferentSrcAddr",
			false,
			&FiveTuple{UDP, srcAddr1, dstAddr1},
			&FiveTuple{UDP, srcAddr2, dstAddr1},
		},
		{
			"DifferentDstAddr",
			false,
			&FiveTuple{UDP, srcAddr1, dstAddr1},
			&FiveTuple{UDP, srcAddr1, dstAddr2},
		},
	}

	for _, tc := range tt {
		a := tc.a
		b := tc.b
		expect := tc.expect

		t.Run(tc.name, func(t *testing.T) {
			fact := a.Equal(b)

			if expect != fact {
				t.Errorf("%v, %v equal check should be %t, but %t", a, b, expect, fact)
			}
		})
	}
}
