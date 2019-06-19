package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/gortc/turn"
)

func TestChannelBind(t *testing.T) {
	c := newChannelBind(2 * time.Second)
	if c.allocation.GetChannelByID(c.ID) != c {
		t.Errorf("GetChannelByID(%d) shouldn't be nil after added to allocation", c.ID)
	}
}

func TestChannelBindStart(t *testing.T) {
	c := newChannelBind(2 * time.Second)
	time.Sleep(3 * time.Second)
	if c.allocation.GetChannelByID(c.ID) != nil {
		t.Errorf("GetChannelByID(%d) should be nil if timeout", c.ID)
	}
}

func TestChannelBindReset(t *testing.T) {
	c := newChannelBind(3 * time.Second)
	time.Sleep(2 * time.Second)
	c.refresh(3 * time.Second)
	time.Sleep(2 * time.Second)
	if c.allocation.GetChannelByID(c.ID) == nil {
		t.Errorf("GetChannelByID(%d) shouldn't be nil after refresh", c.ID)
	}
}

func newChannelBind(lifetime time.Duration) *ChannelBind {
	allocation := &Allocation{}

	addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	c := &ChannelBind{
		ID:   turn.ChannelNumber(turn.MinChannelNumber + 1),
		Peer: addr,
	}

	_ = allocation.AddChannelBind(c, lifetime)

	return c
}
