// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBindingManager(t *testing.T) {
	t.Run("number assignment", func(t *testing.T) {
		bm := newBindingManager()
		var chanNum uint16
		for i := uint16(0); i < 10; i++ {
			chanNum = bm.assignChannelNumber()
			assert.Equal(t, minChannelNumber+i, chanNum, "should match")
		}

		bm.next = uint16(0x7ff0)
		for i := uint16(0); i < 16; i++ {
			chanNum = bm.assignChannelNumber()
			assert.Equal(t, 0x7ff0+i, chanNum, "should match")
		}

		// Back to min
		chanNum = bm.assignChannelNumber()
		assert.Equal(t, minChannelNumber, chanNum, "should match")
	})

	t.Run("method test", func(t *testing.T) {
		lo := net.IPv4(127, 0, 0, 1)
		count := 100
		bm := newBindingManager()
		for i := 0; i < count; i++ {
			addr := &net.UDPAddr{IP: lo, Port: 10000 + i}
			b0 := bm.create(addr)
			b1, ok := bm.findByAddr(addr)
			assert.True(t, ok, "should succeed")
			b2, ok := bm.findByNumber(b0.number)
			assert.True(t, ok, "should succeed")

			assert.Equal(t, b0, b1, "should match")
			assert.Equal(t, b0, b2, "should match")
		}

		all := bm.all()
		for _, b := range all {
			found, ok := bm.findByNumber(b.number)
			assert.True(t, ok, "should exist")
			assert.Equal(t, b, found, "should match")
		}
		assert.Equal(t, count, len(all), "should match")
		assert.Equal(t, count, bm.size(), "should match")
		assert.Equal(t, count, len(bm.addrMap), "should match")

		for i := 0; i < count; i++ {
			addr := &net.UDPAddr{IP: lo, Port: 10000 + i}
			if i%2 == 0 {
				assert.True(t, bm.deleteByAddr(addr), "should return true")
			} else {
				assert.True(t, bm.deleteByNumber(minChannelNumber+uint16(i)), "should return true") // nolint:gosec // G115
			}
		}

		assert.Equal(t, 0, bm.size(), "should match")
		assert.Equal(t, 0, len(bm.addrMap), "should match")
		assert.Equal(t, 0, len(bm.all()), "should match")
	})

	t.Run("failure test", func(t *testing.T) {
		addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 7777}
		m := newBindingManager()
		var ok bool
		_, ok = m.findByAddr(addr)
		assert.False(t, ok, "should fail")
		_, ok = m.findByNumber(uint16(5555))
		assert.False(t, ok, "should fail")
		ok = m.deleteByAddr(addr)
		assert.False(t, ok, "should fail")
		ok = m.deleteByNumber(uint16(5555))
		assert.False(t, ok, "should fail")
	})
}
