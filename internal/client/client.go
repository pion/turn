package client

import (
	"fmt"
	"net"

	"sync"

	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// Client is a STUN server client
type Client struct {
	conn *ipv4.PacketConn
	mux  *sync.Mutex
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on, default "0.0.0.0:0"
func NewClient(listeningAddress string) (*Client, error) {
	c, err := net.ListenPacket("udp4", listeningAddress)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to listen on %s", listeningAddress))
	}
	conn := ipv4.NewPacketConn(c)
	if err := conn.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return nil, errors.Wrap(err, "failed to SetControlMessage ipv4.FlagDst")
	}

	return &Client{conn, &sync.Mutex{}}, nil

}
