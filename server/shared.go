package server

import (
	"net"
	"strconv"

	"github.com/pkg/errors"
	"gitlab.com/pions/pion/pkg/go/stun"
	"golang.org/x/net/ipv4"
)

func buildAndSend(conn *ipv4.PacketConn, addr net.Addr, class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
	rsp, err := stun.Build(class, method, transactionID, attrs...)
	if err != nil {
		return err
	}

	b := rsp.Pack()
	l, err := conn.WriteTo(b, nil, addr)
	if err != nil {
		return errors.Wrap(err, "failed writing to socket")
	}

	if l != len(b) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected)", l, len(b))
	}

	return nil
}

func netAddrIPPort(addr net.Addr) (net.IP, int, error) {
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, 0, err
	}

	return net.ParseIP(host), port, nil
}
