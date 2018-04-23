package main

import (
	"log"
	"os"

	"github.com/pions/turn"
)

type MyTurnServer struct {
}

func main() {
	if os.Getenv("REALM") == "" {
		log.Panic("REALM is a required environment variable")
	}

	turn.Start(&MyTurnServer{}, os.Getenv("realm"))
}
