// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/pion/turn/v5/internal/proto"
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
