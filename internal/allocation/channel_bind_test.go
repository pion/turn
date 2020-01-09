package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/pion/turn/v2/internal/proto"
)

func TestChannelBind(t *testing.T) {
	c := newChannelBind(2 * time.Second)

	if c.allocation.GetChannelByNumber(c.Number) != c {
		t.Errorf("GetChannelByNumber(%d) shouldn't be nil after added to allocation", c.Number)
	}
}

func TestChannelBindStart(t *testing.T) {
	c := newChannelBind(2 * time.Second)

	time.Sleep(3 * time.Second)

	if c.allocation.GetChannelByNumber(c.Number) != nil {
		t.Errorf("GetChannelByNumber(%d) should be nil if timeout", c.Number)
	}
}

func TestChannelBindReset(t *testing.T) {
	c := newChannelBind(3 * time.Second)

	time.Sleep(2 * time.Second)
	c.refresh(3 * time.Second)
	time.Sleep(2 * time.Second)

	if c.allocation.GetChannelByNumber(c.Number) == nil {
		t.Errorf("GetChannelByNumber(%d) shouldn't be nil after refresh", c.Number)
	}
}

func newChannelBind(lifetime time.Duration) *ChannelBind {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:0")
	c := &ChannelBind{
		Number: proto.MinChannelNumber,
		Peer:   addr,
	}

	_ = a.AddChannelBind(c, lifetime)

	return c
}
