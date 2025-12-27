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

func TestFiveTuple_MarshalUnmarshalBinary(t *testing.T) {
	t.Run("UDP", func(t *testing.T) {
		srcAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:1234")
		dstAddr, _ := net.ResolveUDPAddr("udp", "192.168.1.1:5678")
		original := &FiveTuple{
			Protocol: UDP,
			SrcAddr:  srcAddr,
			DstAddr:  dstAddr,
		}

		data, err := original.MarshalBinary()
		assert.NoError(t, err)

		unmarshaled := &FiveTuple{}
		err = unmarshaled.UnmarshalBinary(data)
		assert.NoError(t, err)

		assert.Equal(t, original.Protocol, unmarshaled.Protocol)
		assert.Equal(t, original.SrcAddr.String(), unmarshaled.SrcAddr.String())
		assert.Equal(t, original.DstAddr.String(), unmarshaled.DstAddr.String())
	})

	t.Run("TCP", func(t *testing.T) {
		srcAddr, _ := net.ResolveTCPAddr("tcp", "10.0.0.1:1111")
		dstAddr, _ := net.ResolveTCPAddr("tcp", "10.0.0.2:2222")
		original := &FiveTuple{
			Protocol: TCP,
			SrcAddr:  srcAddr,
			DstAddr:  dstAddr,
		}

		data, err := original.MarshalBinary()
		assert.NoError(t, err)

		unmarshaled := &FiveTuple{}
		err = unmarshaled.UnmarshalBinary(data)
		assert.NoError(t, err)

		assert.Equal(t, original.Protocol, unmarshaled.Protocol)
		assert.Equal(t, original.SrcAddr.String(), unmarshaled.SrcAddr.String())
		assert.Equal(t, original.DstAddr.String(), unmarshaled.DstAddr.String())
	})
}
