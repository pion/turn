package main

import (
	"errors"
	"flag"
	"log"
	"net"

	"github.com/pion/logging"
	"github.com/pion/turn"
)

func main() {
	host := flag.String("host", "74.125.143.127", "IP of TURN Server. Default is the IP of stun1.l.google.com.")
	port := flag.Int("port", 19302, "Port of TURN server.")
	flag.Parse()

	ip := net.ParseIP(*host)
	if ip == nil {
		panic(errors.New("failed to parse host IP"))
	}

	c, err := turn.NewClient(&turn.ClientConfig{
		ListeningAddress: "0.0.0.0:0",
		LoggerFactory:    logging.NewDefaultLoggerFactory(),
	})
	if err != nil {
		panic(err)
	}

	mappedAddr, err := c.SendSTUNRequest(ip, *port)
	if err != nil {
		panic(err)
	}

	log.Printf("mapped-address=%s:%s",
		mappedAddr.Network(),
		mappedAddr.String())
}
