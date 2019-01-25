package allocation

import "github.com/pions/stun"

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
	SrcAddr *stun.TransportAddr
	DstAddr *stun.TransportAddr
}

// Equal asserts if two FiveTuples are equal
func (f *FiveTuple) Equal(b *FiveTuple) bool {
	return f.SrcAddr.Equal(b.SrcAddr) &&
		f.DstAddr.Equal(b.DstAddr) &&
		f.Protocol == b.Protocol
}
