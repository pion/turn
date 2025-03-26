// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFiveTupleProtocol(t *testing.T) {
	udpExpect := Protocol(0)
	tcpExpect := Protocol(1)
	assert.Equal(t, UDP, udpExpect)
	assert.Equal(t, TCP, tcpExpect)
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
			assert.Equalf(t, expect, fact, "%v, %v equal check should be %t, but %t", a, b, expect, fact)
		})
	}
}
