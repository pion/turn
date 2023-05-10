package client

import (
	"net"

	"github.com/pion/stun"
)

// Client is an interface for the public turn.Client in order to break cyclic dependencies
type Client interface {
	TURNServerAddr() net.Addr
	Username() stun.Username
	Realm() stun.Realm
	WriteTo(data []byte, to net.Addr) (int, error)
	PerformTransaction(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error)
	OnDeallocated(relayedAddr net.Addr)
}
