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

	resp := &stun.Message{Raw: append([]byte{}, packet[:size]...)}
	if err := resp.Decode(); err != nil {
		return nil, errors.Wrap(err, "failed to handle reply")
	}

	var reflAddr stun.XORMappedAddress
	if err := reflAddr.GetFrom(resp); err != nil {
		return nil, err
	}

	return fmt.Sprintf("pkt_size=%d dst_ip=%s src_addr=%s refl_addr=%s:%d", size, cm.Dst, addr, reflAddr.IP, reflAddr.Port), nil
}

func sendStunRequest(conn net.PacketConn, serverIP net.IP, serverPort int) error {
	msg, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(msg.Raw, &net.UDPAddr{IP: serverIP, Port: serverPort})
	return err
}
