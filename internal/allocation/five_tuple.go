package allocation

import (
	"net"

	"github.com/pion/turn/internal/ipnet"
)

// Protocol is an enum for relay protocol
type Protocol int

// Network protocols for relay
const (
	UDP Protocol = iota
	TCP Protocol = iota
)

// FiveTuple is the combination (client IP address and port, server IP
// address and port, and transport protocol (currently one of UDP,
// TCP, or TLS)) used to communicate between the client and the
// server.  The 5-tuple uniquely identifies this communication
// stream.  The 5-tuple also uniquely identifies the Allocation on
// the server.
type FiveTuple struct {
	Protocol
	SrcAddr, DstAddr net.Addr
}

// Equal asserts if two FiveTuples are equal
func (f *FiveTuple) Equal(b *FiveTuple) bool {
	return ipnet.AddrEqual(f.SrcAddr, b.SrcAddr) && ipnet.AddrEqual(f.DstAddr, b.DstAddr)
}
