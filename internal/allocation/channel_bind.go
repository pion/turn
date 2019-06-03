package allocation

import (
	"fmt"
	"net"
	"time"
)

const channelBindTimeout = time.Duration(10) * time.Minute

// ChannelBind represents a TURN Channel
// https://tools.ietf.org/html/rfc5766#section-2.5
type ChannelBind struct {
	Peer          net.Addr
	ID            uint16
	allocation    *Allocation
	lifetimeTimer *time.Timer
}

func (c *ChannelBind) start() {
	c.lifetimeTimer = time.AfterFunc(channelBindTimeout, func() {
		if !c.allocation.RemoveChannelBind(c.ID) {
			fmt.Printf("Failed to remove ChannelBind for %v %x %v \n", c.ID, c.Peer, c.allocation.fiveTuple)
		}
	})
}

func (c *ChannelBind) refresh() {
	if !c.lifetimeTimer.Reset(channelBindTimeout) {
		fmt.Printf("Failed to reset ChannelBind timer for %v %x %v \n", c.ID, c.Peer, c.allocation.fiveTuple)
	}
}
