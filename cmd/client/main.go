package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/pion/logging"
	"github.com/pion/turn"
)

func main() {
	host := flag.String("host", "stun1.l.google.com", "IP of TURN Server. Default is stun1.l.google.com.")
	port := flag.Int("port", 19302, "Port of TURN server.")
	software := flag.String("software", "", "The STUN SOFTWARE attribute. Useful for debugging purpose.")
	flag.Parse()

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err2 := conn.Close(); err2 != nil {
			panic(err2)
		}
	}()

	cfg := &turn.ClientConfig{
		STUNServerAddr: fmt.Sprintf("%s:%d", *host, *port),
		Software:       *software,
		Conn:           conn,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	}

	c, err := turn.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	err = c.Listen()
	if err != nil {
		panic(err)
	}

	mappedAddr, err := c.SendBindingRequest()
	if err != nil {
		panic(err)
	}

	log.Printf("mapped-address=%s:%s",
		mappedAddr.Network(),
		mappedAddr.String())
}
