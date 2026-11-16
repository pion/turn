// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"

	"github.com/pion/turn/v6/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestBindingManager(t *testing.T) {
	channelNumberRange := proto.ChannelNumberRange{Min: 0x4000, Max: 0x7FFF}

	t.Run("number assignment", func(t *testing.T) {
		tests := []struct {
			name string
			rng  proto.ChannelNumberRange
		}{
			{"default range", channelNumberRange},
			{"narrow range from default min", proto.ChannelNumberRange{Min: 0x4000, Max: 0x4002}},
			{"range not starting at default min", proto.ChannelNumberRange{Min: 0x4100, Max: 0x4103}},
			{"single channel", proto.ChannelNumberRange{Min: 0x4200, Max: 0x4200}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bm := newBindingManager(tt.rng)
				size := int(tt.rng.Max-tt.rng.Min) + 1

				// Check wraparound over two full cycles.
				for i := range 2 * size {
					want := uint16(tt.rng.Min) + uint16(i%size) // nolint:gosec // G115
					assert.Equal(t, want, bm.assignChannelNumber(), "should match")
				}
			})
		}
	})

	t.Run("method test", func(t *testing.T) {
		lo := net.IPv4(127, 0, 0, 1)
		count := 100
		bm := newBindingManager(channelNumberRange)
		for i := range count {
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

		for i := range count {
			addr := &net.UDPAddr{IP: lo, Port: 10000 + i}
			if i%2 == 0 {
				assert.True(t, bm.deleteByAddr(addr), "should return true")
			} else {
				assert.True(t, bm.deleteByNumber(uint16(channelNumberRange.Min)+uint16(i)), "should return true") // nolint:gosec // G115
			}
		}

		assert.Equal(t, 0, bm.size(), "should match")
		assert.Equal(t, 0, len(bm.addrMap), "should match")
		assert.Equal(t, 0, len(bm.all()), "should match")
	})

	t.Run("failure test", func(t *testing.T) {
		addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 7777}
		m := newBindingManager(channelNumberRange)
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
