package allocation

import (
	"fmt"
	"net"
	"time"

	"github.com/gortc/turn"
)

// ChannelBind represents a TURN Channel
// https://tools.ietf.org/html/rfc5766#section-2.5
type ChannelBind struct {
	Peer          net.Addr
	ID            turn.ChannelNumber
	allocation    *Allocation
	lifetimeTimer *time.Timer
}

func (c *ChannelBind) start(lifetime time.Duration) {
	c.lifetimeTimer = time.AfterFunc(lifetime, func() {
		if !c.allocation.RemoveChannelBind(c.ID) {
			fmt.Printf("Failed to remove ChannelBind for %v %x %v \n", c.ID, c.Peer, c.allocation.fiveTuple)
		}
	})
}

func (c *ChannelBind) refresh(lifetime time.Duration) {
	if !c.lifetimeTimer.Reset(lifetime) {
		fmt.Printf("Failed to reset ChannelBind timer for %v %x %v \n", c.ID, c.Peer, c.allocation.fiveTuple)
	}
}
