package client

import (
	"fmt"
	"net"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// SendSTUNRequest sends a new STUN request to the serverIP with serverPort
func (c *Client) SendSTUNRequest(serverIP net.IP, serverPort int) (interface{}, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	packet := make([]byte, 1500)
	if err := sendStunRequest(c.conn, serverIP, serverPort); err != nil {
		return nil, err
	}
	size, cm, addr, err := c.conn.ReadFrom(packet)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read packet from udp socket")
	}
	srcAddr, err := stun.NewTransportAddr(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed reading udp addr")
	}

	return fmt.Sprintf("pkt_size=%d %s src_address=%s", size, cm.String(), srcAddr.String()), nil
}

func sendStunRequest(conn *ipv4.PacketConn, serverIP net.IP, serverPort int) error {
	serverAddress := stun.TransportAddr{IP: serverIP, Port: serverPort}
	return stun.BuildAndSend(conn, &serverAddress, stun.ClassRequest, stun.MethodBinding, stun.GenerateTransactionId())
}
