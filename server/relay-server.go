package server

import "net"

type RelayServer struct {
	clientAddress *net.UDPAddr
}

func (r *RelayServer) isAllocated(addr *net.UDPAddr) bool {
	panic("not implemented")
}

func relayIsAllocated() bool {
	return false
}
