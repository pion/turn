package allocation

import (
	"fmt"
	"time"

	"github.com/pions/pkg/stun"
)

const channelBindTimeout = time.Duration(10) * time.Minute

type ChannelBind struct {
	Peer          *stun.TransportAddr
	Id            uint16
	allocation    *Allocation
	lifetimeTimer *time.Timer
}

func (c *ChannelBind) start() {
	c.lifetimeTimer = time.AfterFunc(channelBindTimeout, func() {
		if !c.allocation.RemoveChannelBind(c.Id) {
			fmt.Printf("Failed to remove ChannelBind for %v %x %v", c.Id, c.Peer, c.allocation.fiveTuple)
		}
	})
}

func (c *ChannelBind) refresh() {
	if !c.lifetimeTimer.Reset(channelBindTimeout) {
		fmt.Printf("Failed to reset ChannelBind timer for %v %x %v", c.Id, c.Peer, c.allocation.fiveTuple)
	}
}
