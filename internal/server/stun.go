package server

import (
	"net"

	"github.com/pion/stun"
	"github.com/pion/turn/internal/ipnet"
)

func (s *Server) handleBindingRequest(srcAddr, dstAddr net.Addr, m *stun.Message) error {
	ip, port, err := ipnet.AddrIPPort(srcAddr)
	if err != nil {
		return err
	}

	return buildAndSend(s.connection, srcAddr, &stun.Message{TransactionID: m.TransactionID}, stun.BindingSuccess,
		&stun.XORMappedAddress{
			IP:   ip,
			Port: port,
		},
		stun.Fingerprint,
	)
}
