package client

import (
	"fmt"
	"net"

	"github.com/pion/stun"
	"github.com/pkg/errors"
)

// SendSTUNRequest sends a new STUN request to the serverIP with serverPort
func (c *Client) SendSTUNRequest(serverIP net.IP, serverPort int) (interface{}, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	if err := sendStunRequest(c.conn, serverIP, serverPort); err != nil {
		return nil, err
	}

	packet := make([]byte, 1500)
	size, cm, addr, err := c.conn.ReadFromCM(packet)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read packet from udp socket")
	}
	srcAddr, err := stun.NewTransportAddr(addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed reading udp addr")
	}

	resp, err := stun.NewMessage(packet[:size])
	if err != nil {
		return nil, errors.Wrap(err, "failed to handle reply")
	}

	attr, ok := resp.GetOneAttribute(stun.AttrXORMappedAddress)
	if !ok {
		return nil, errors.Errorf("got respond from STUN server that did not contain XORAddress")
	}

	var reflAddr stun.XorAddress
	if err = reflAddr.Unpack(resp, attr); err != nil {
		return nil, errors.Wrapf(err, "failed to unpack STUN XorAddress response")
	}

	return fmt.Sprintf("pkt_size=%d dst_ip=%s src_addr=%s refl_addr=%s:%d", size, cm.Dst, srcAddr, reflAddr.IP, reflAddr.Port), nil
}

func sendStunRequest(conn net.PacketConn, serverIP net.IP, serverPort int) error {
	serverAddress := stun.TransportAddr{IP: serverIP, Port: serverPort}
	return stun.BuildAndSend(conn, &serverAddress, stun.ClassRequest, stun.MethodBinding, stun.GenerateTransactionID())
}
