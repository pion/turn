package allocation

import (
	"fmt"
	"net"
	"net/netip"
)

// Protocol is an enum for relay protocol
type Protocol uint8

// Network protocols for relay
const (
	UDP Protocol = iota
	TCP
)

// FiveTuple is the combination (client IP address and port, server IP
// address and port, and transport protocol (currently one of UDP,
// TCP, or TLS)) used to communicate between the client and the
// server.  The 5-tuple uniquely identifies this communication
// stream.  The 5-tuple also uniquely identifies the Allocation on
// the server.
type FiveTuple struct {
	Protocol
	SrcAddr, DstAddr netip.AddrPort
}

// NewFiveTuple returns a new FiveTuple taking a net.Addr for both the source and the destination
// address.
func NewFiveTuple(srcAddr, dstAddr net.Addr, proto Protocol) *FiveTuple {
	var s, d netip.AddrPort

	switch a := srcAddr.(type) {
	case *net.UDPAddr:
		s = a.AddrPort()
	case *net.TCPAddr:
		s = a.AddrPort()
	}

	switch a := dstAddr.(type) {
	case *net.UDPAddr:
		d = a.AddrPort()
	case *net.TCPAddr:
		d = a.AddrPort()
	}

	return &FiveTuple{SrcAddr: s, DstAddr: d, Protocol: proto}
}

// Equal asserts if two FiveTuples are equal
func (f *FiveTuple) Equal(b *FiveTuple) bool {
	return f.Protocol == b.Protocol && f.SrcAddr == b.SrcAddr && f.DstAddr == b.DstAddr
}

// Fingerprint is the identity of a FiveTuple
func (f *FiveTuple) Fingerprint() string {
	return fmt.Sprintf("%d_%s_%s", f.Protocol, f.SrcAddr.String(), f.DstAddr.String())
}
