package server

import (
	"github.com/pion/stun"
)

func (s *Server) handleBindingRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	_ = dstAddr // Silence linter
	return stun.BuildAndSend(s.connection, srcAddr, stun.ClassSuccessResponse, stun.MethodBinding, m.TransactionID,
		&stun.XorMappedAddress{
			XorAddress: stun.XorAddress{
				IP:   srcAddr.IP,
				Port: srcAddr.Port,
			},
		},
		&stun.Fingerprint{},
	)
}
