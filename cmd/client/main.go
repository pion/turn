package main

import (
	"log"
	"net"

	"github.com/pions/turn"
)

func main() {

	args := turn.ClientArguments{
		BindingAddress: "0.0.0.0:0",
		// IP and port of stun1.l.google.com
		ServerIP:   net.IPv4(74, 125, 143, 127),
		ServerPort: 19302,
	}

	resp, err := turn.StartClient(args)
	if err != nil {
		panic(err)
	}

	log.Println(resp)

}
