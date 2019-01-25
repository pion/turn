package ipnet

import "net"

// A ControlMessage represents per packet basis IP-level socket options.
type ControlMessage struct {
	Dst net.IP // destination address, receiving only
}
