package allocation

import (
	"net"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/logging"
)

// ChannelBind represents a TURN Channel
// https://tools.ietf.org/html/rfc5766#section-2.5
type ChannelBind struct {
	Peer   net.Addr
	Number turn.ChannelNumber

	allocation    *Allocation
	lifetimeTimer *time.Timer
	log           logging.LeveledLogger
}

// NewChannelBind creates a new ChannelBind
func NewChannelBind(number turn.ChannelNumber, peer net.Addr, log logging.LeveledLogger) *ChannelBind {
	return &ChannelBind{
		Number: number,
		Peer:   peer,
		log:    log,
	}
}

func (c *ChannelBind) start(lifetime time.Duration) {
	c.lifetimeTimer = time.AfterFunc(lifetime, func() {
		c.allocation.RemoveChannelBind(c)
	})
}

func (c *ChannelBind) refresh(lifetime time.Duration) {
	if !c.lifetimeTimer.Reset(lifetime) {
		c.log.Errorf("Failed to reset ChannelBind timer for %v %x %v", c.Number, c.Peer, c.allocation.fiveTuple)
	}
}
