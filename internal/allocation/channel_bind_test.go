package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/gortc/turn"
)

func TestChannelBind(t *testing.T) {
	c := newChannelBind(2 * time.Second)

	if c.allocation.GetChannelByID(c.Number) != c {
		t.Errorf("GetChannelByID(%d) shouldn't be nil after added to allocation", c.Number)
	}
}

func TestChannelBindStart(t *testing.T) {
	c := newChannelBind(2 * time.Second)

	time.Sleep(3 * time.Second)

	if c.allocation.GetChannelByID(c.Number) != nil {
		t.Errorf("GetChannelByID(%d) should be nil if timeout", c.Number)
	}
}

func TestChannelBindReset(t *testing.T) {
	c := newChannelBind(3 * time.Second)

	time.Sleep(2 * time.Second)
	c.refresh(3 * time.Second)
	time.Sleep(2 * time.Second)

	if c.allocation.GetChannelByID(c.Number) == nil {
		t.Errorf("GetChannelByID(%d) shouldn't be nil after refresh", c.Number)
	}
}

func newChannelBind(lifetime time.Duration) *ChannelBind {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	c := &ChannelBind{
		Number: turn.ChannelNumber(turn.MinChannelNumber + 1),
		Peer:   addr,
	}

	_ = a.AddChannelBind(c, lifetime)

	return c
}
