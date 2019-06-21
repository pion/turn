package turn

import (
	"net"

	"github.com/pion/stun"
	"github.com/pion/turn/internal/ipnet"
)

// caller must hold the mutex
func (s *Server) handleBindingRequest(conn net.PacketConn, srcAddr net.Addr, m *stun.Message) error {
	ip, port, err := ipnet.AddrIPPort(srcAddr)
	if err != nil {
		return err
	}

	return buildAndSend(conn, srcAddr, &stun.Message{TransactionID: m.TransactionID}, stun.BindingSuccess,
		&stun.XORMappedAddress{
			IP:   ip,
			Port: port,
		},
		stun.Fingerprint,
	)
}
