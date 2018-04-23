package main

import (
	"log"
	"os"

	"github.com/pions/pkg/stun"
	"github.com/pions/turn"
)

type MyTurnServer struct {
}

func (m *MyTurnServer) AuthenticateRequest(username string, srcAddr *stun.TransportAddr) (password string, ok bool) {
	return "password", true
}

func main() {
	if os.Getenv("REALM") == "" {
		log.Panic("REALM is a required environment variable")
	}

	turn.Start(&MyTurnServer{}, os.Getenv("REALM"))
}
