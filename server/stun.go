package server

import (
	"github.com/pion/stun"
	"github.com/pion/turn/internal/ipnet"
)

// caller must hold the mutex
func (s *Server) handleBindingRequest(ctx *context) error {
	ip, port, err := ipnet.AddrIPPort(ctx.srcAddr)
	if err != nil {
		return err
	}

	return ctx.respond(&stun.XORMappedAddress{
		IP:   ip,
		Port: port,
	}, stun.Fingerprint)
}
