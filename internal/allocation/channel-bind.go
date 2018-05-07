package allocation

import (
	"fmt"

	"github.com/pions/pkg/stun"
)

type ChannelBind struct {
	Peer *stun.TransportAddr
	Id   uint16
	// expiration uint32
}

func (c *ChannelBind) refresh() {
	fmt.Println("Refresh ChannelBind!")
}
