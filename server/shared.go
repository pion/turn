package server

import (
	"net"

	"github.com/pkg/errors"
	"gitlab.com/pions/pion/pkg/go/stun"
)

func buildAndSend(conn *net.UDPConn, addr *net.UDPAddr, class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
	rsp, err := stun.Build(class, method, transactionID, attrs...)
	if err != nil {
		return err
	}

	b := rsp.Pack()
	l, err := conn.WriteTo(b, addr)
	if err != nil {
		return errors.Wrap(err, "failed writing to socket")
	}

	if l != len(b) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected)", l, len(b))
	}

	return nil
}
