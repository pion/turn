package main

import (
	"errors"
	"flag"
	"log"
	"net"

	"github.com/pions/turn"
)

func main() {
	host := flag.String("host", "74.125.143.127", "IP of TURN Server. Default is the IP of stun1.l.google.com.")
	port := flag.Int("port", 19302, "Port of TURN server.")
	flag.Parse()

	ip := net.ParseIP(*host)
	if ip == nil {
		panic(errors.New("failed to parse host IP"))
	}

	args := turn.ClientArguments{
		BindingAddress: "0.0.0.0:0",
		ServerIP:       ip,
		ServerPort:     *port,
	}

	resp, err := turn.StartClient(args)
	if err != nil {
		panic(err)
	}

	log.Println(resp)
}
