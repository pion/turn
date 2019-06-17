package turn

import (
	"fmt"
	"net"

	"sync"

	"github.com/pion/logging"
	"github.com/pkg/errors"
)

// ClientConfig is a bag of config parameters for Client.
type ClientConfig struct {
	ListeningAddress string
	LoggerFactory    logging.LoggerFactory
}

// Client is a STUN server client
type Client struct {
	conn net.PacketConn
	mux  sync.Mutex
	log  logging.LeveledLogger
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on, default "0.0.0.0:0"
func NewClient(config *ClientConfig) (*Client, error) {
	log := config.LoggerFactory.NewLogger("turnc")
	network := "udp4"

	conn, err := net.ListenPacket(network, config.ListeningAddress)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to listen on %s", config.ListeningAddress))
	}

	return &Client{
		conn: conn,
		log:  log,
	}, nil
}
