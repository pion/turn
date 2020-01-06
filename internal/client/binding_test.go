package client

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBindingManager(t *testing.T) {
	t.Run("number assignment", func(t *testing.T) {
		m := newBindingManager()
		var n uint16
		for i := uint16(0); i < 10; i++ {
			n = m.assignChannelNumber()
			assert.Equal(t, minChannelNumber+i, n, "should match")
		}

		m.next = uint16(0x7ff0)
		for i := uint16(0); i < 16; i++ {
			n = m.assignChannelNumber()
			assert.Equal(t, 0x7ff0+i, n, "should match")
		}
		// back to min
		n = m.assignChannelNumber()
		assert.Equal(t, minChannelNumber, n, "should match")
	})

	t.Run("method test", func(t *testing.T) {
		lo := net.IPv4(127, 0, 0, 1)
		count := 100
		m := newBindingManager()
		for i := 0; i < count; i++ {
			addr := &net.UDPAddr{IP: lo, Port: 10000 + i}
			b0 := m.create(addr)
			b1, ok := m.findByAddr(addr)
			assert.True(t, ok, "should succeed")
			b2, ok := m.findByNumber(b0.number)
			assert.True(t, ok, "should succeed")

			assert.Equal(t, b0, b1, "should match")
			assert.Equal(t, b0, b2, "should match")
		}

		assert.Equal(t, count, m.size(), "should match")
		assert.Equal(t, count, len(m.addrMap), "should match")

		for i := 0; i < count; i++ {
			addr := &net.UDPAddr{IP: lo, Port: 10000 + i}
			if i%2 == 0 {
				assert.True(t, m.deleteByAddr(addr), "should return true")
			} else {
				assert.True(t, m.deleteByNumber(minChannelNumber+uint16(i)), "should return true")
			}
		}

		assert.Equal(t, 0, m.size(), "should match")
		assert.Equal(t, 0, len(m.addrMap), "should match")
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
