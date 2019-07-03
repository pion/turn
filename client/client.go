package client

import (
	"fmt"
	"net"
	"sync"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/transport/vnet"
	"github.com/pkg/errors"
)

// Config is a bag of config parameters for Client.
type Config struct {
	ListeningAddress string
	LoggerFactory    logging.LoggerFactory
	Net              *vnet.Net
}

// Client is a STUN server client
type Client struct {
	conn net.PacketConn
	mux  sync.Mutex
	net  *vnet.Net
	log  logging.LeveledLogger
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on, default "0.0.0.0:0"
func NewClient(config *Config) (*Client, error) {
	log := config.LoggerFactory.NewLogger("turnc")
	network := "udp4"

	if config.Net == nil {
		config.Net = vnet.NewNet(nil) // defaults to native operation
	} else {
		log.Warn("vnet is enabled")
	}

	c := &Client{
		net: config.Net,
		log: log,
	}

	var err error
	c.conn, err = c.net.ListenPacket(network, config.ListeningAddress)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to listen on %s", config.ListeningAddress))
	}

	return c, nil
}

// SendSTUNRequest sends a new STUN request to the serverIP with serverPort
func (c *Client) SendSTUNRequest(serverIP net.IP, serverPort int) (net.Addr, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.log.Debug("sending STUN request")
	if err := sendStunRequest(c.conn, serverIP, serverPort); err != nil {
		return nil, err
	}

	packet := make([]byte, 1500)
	c.log.Debug("wait for STUN response...")
	size, _, err := c.conn.ReadFrom(packet)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read packet from udp socket")
	}

	c.log.Debugf("received %d bytes of STUN response", size)
	resp := &stun.Message{Raw: append([]byte{}, packet[:size]...)}
	if err := resp.Decode(); err != nil {
		return nil, errors.Wrap(err, "failed to handle reply")
	}

	var reflAddr stun.XORMappedAddress
	if err := reflAddr.GetFrom(resp); err != nil {
		return nil, err
	}

	//return fmt.Sprintf("pkt_size=%d src_addr=%s refl_addr=%s:%d", size, addr, reflAddr.IP, reflAddr.Port), nil
	return &net.UDPAddr{
		IP:   reflAddr.IP,
		Port: reflAddr.Port,
	}, nil
}

func sendStunRequest(conn net.PacketConn, serverIP net.IP, serverPort int) error {
	msg, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		return err
	}
	_, err = conn.WriteTo(msg.Raw, &net.UDPAddr{IP: serverIP, Port: serverPort})
	return err
}
