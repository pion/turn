package relayServer

import (
	"net"
	"strconv"
)

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
