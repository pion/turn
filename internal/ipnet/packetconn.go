package ipnet

import (
	"fmt"
	"net"
	"strings"
)

// A PacketConn represents a packet network endpoint that uses the IPvX transport.
type PacketConn interface {
	net.PacketConn

	ReadFromCM(b []byte) (n int, cm *ControlMessage, src net.Addr, err error)
}

// NewPacketConn returns a new PacketConn using c as its underlying transport.
func NewPacketConn(network string, c net.PacketConn) (PacketConn, error) {
	switch strings.ToLower(network) {
	case "udp4":
		return newIPv4PacketConn(c)
	default:
		return nil, fmt.Errorf("unsupported network type: %s", network)
	}
}
