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

func randomFreePort(protocol string) (port int, err error) {
	addr, err := net.ResolveTCPAddr(protocol, "localhost:0")
	if err != nil {
		return
	}

	l, err := net.ListenTCP(protocol, addr)
	if err != nil {
		return
	}

	port = l.Addr().(*net.TCPAddr).Port
	err = l.Close()
	return
}
