package allocation

import "github.com/pions/pkg/stun"

type Protocol int

const (
	UDP Protocol = iota
	TCP Protocol = iota
)

type FiveTuple struct {
	SrcAddr  *stun.TransportAddr
	DstAddr  *stun.TransportAddr
	Protocol Protocol
}

func (f *FiveTuple) Equal(b *FiveTuple) bool {
	return f.SrcAddr.Equal(b.SrcAddr) &&
		f.DstAddr.Equal(b.DstAddr) &&
		f.Protocol == b.Protocol
}
