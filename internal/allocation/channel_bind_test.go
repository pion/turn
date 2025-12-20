// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestChannelBind(t *testing.T) {
	c := newChannelBind(2 * time.Second)
	assert.Equalf(t, c, c.allocation.GetChannelByNumber(c.Number),
		"GetChannelByNumber(%d) shouldn't be nil after added to allocation", c.Number)
}

func TestChannelBindStart(t *testing.T) {
	c := newChannelBind(2 * time.Second)

	time.Sleep(3 * time.Second)
	assert.Nil(t, c.allocation.GetChannelByNumber(c.Number),
		"GetChannelByNumber(%d) should be nil after timeout", c.Number)
}

func TestChannelBindReset(t *testing.T) {
	c := newChannelBind(3 * time.Second)

	time.Sleep(2 * time.Second)
	c.refresh(3 * time.Second)
	time.Sleep(2 * time.Second)
	assert.NotNil(t, c.allocation.GetChannelByNumber(c.Number),
		"GetChannelByNumber(%d) shouldn't be nil after refresh", c.Number)
}

func newChannelBind(lifetime time.Duration) *ChannelBind {
	a := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	c := &ChannelBind{
		Number: proto.MinChannelNumber,
		Peer:   addr,
	}

	_ = a.AddChannelBind(c, lifetime, DefaultPermissionTimeout)

	return c
}

func TestChannelBind_MarshalUnmarshalBinary(t *testing.T) {
	mockFiveTuple := &FiveTuple{
		Protocol: UDP,
		SrcAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
		DstAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678},
	}
	mockAllocation := NewAllocation(nil, mockFiveTuple, EventHandler{}, logging.NewDefaultLeveledLoggerForScope("test", logging.LogLevelTrace, nil))

	t.Run("UDP", func(t *testing.T) {
		addr, _ := net.ResolveUDPAddr("udp", "192.168.1.100:1000")
		original := NewChannelBind(proto.MinChannelNumber, addr, mockAllocation.log)
		original.allocation = mockAllocation
		original.start(5 * time.Second)

		data, err := original.MarshalBinary()
		assert.NoError(t, err)

		unmarshaled := &ChannelBind{
			allocation: mockAllocation,
			log:        mockAllocation.log,
		}
		err = unmarshaled.UnmarshalBinary(data)
		assert.NoError(t, err)

		assert.Equal(t, original.Peer.String(), unmarshaled.Peer.String())
		assert.Equal(t, original.Number, unmarshaled.Number)
		assert.NotNil(t, unmarshaled.lifetimeTimer, "lifetimeTimer should be restarted after unmarshal")
		assert.WithinDuration(t, original.expiresAt, unmarshaled.expiresAt, 10*time.Millisecond, "expiresAt should be preserved")

		unmarshaled.lifetimeTimer.Stop()
	})
}
