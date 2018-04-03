package main

import (
	"fmt"
	"os"

	"gitlab.com/pions/pion/pkg/go/log"
	"gitlab.com/pions/pion/turn/internal/stun"
)

func main() {
	if os.Getenv("FQDN") == "" {
		log.Fatal().Msg("FQDN is a required environment variable")
	}

	s := stunServer.NewTurnServer()
	errors := make(chan error)

	go func() {
		err := s.Listen("", stunServer.DefaultPort)
		errors <- err
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
