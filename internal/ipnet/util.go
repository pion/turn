// Package ipnet contains helper functions around net and IP
package ipnet

import (
	"fmt"
	"net"
)

// AddrIPPort extracts the IP and Port from a net.Addr
func AddrIPPort(a net.Addr) (net.IP, int, error) {
	aUDP, ok := a.(*net.UDPAddr)
	if ok {
		return aUDP.IP, aUDP.Port, nil
	}

	aTCP, ok := a.(*net.TCPAddr)
	if ok {
		return aTCP.IP, aTCP.Port, nil
	}

	return nil, 0, fmt.Errorf("failed to cast net.Addr to *net.UDPAddr or *net.TCPAddr")
}

// AddrEqual asserts that two net.Addrs are equal
// Currently only supprots UDP and TCP
func AddrEqual(a, b net.Addr) bool {
	switch a := a.(type) {
	case *net.UDPAddr:
		bUDP, ok := b.(*net.UDPAddr)
		if !ok {
			return false
		}
		return a.IP.Equal(bUDP.IP) && a.Port == bUDP.Port
	case *net.TCPAddr:
		bTCP, ok := b.(*net.TCPAddr)
		if !ok {
			return false
		}
		return a.IP.Equal(bTCP.IP) && a.Port == bTCP.Port
	default:
		return false
	}
}
