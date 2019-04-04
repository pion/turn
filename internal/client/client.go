package client

import (
	"fmt"
	"net"

	"sync"

	"github.com/pion/turn/internal/ipnet"
	"github.com/pkg/errors"
)

// Client is a STUN server client
type Client struct {
	conn ipnet.PacketConn
	mux  *sync.Mutex
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on, default "0.0.0.0:0"
func NewClient(listeningAddress string) (*Client, error) {
	network := "udp4"
	c, err := net.ListenPacket(network, listeningAddress)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to listen on %s", listeningAddress))
	}
	conn, err := ipnet.NewPacketConn(network, c)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create connection")
	}

	return &Client{conn, &sync.Mutex{}}, nil
}
