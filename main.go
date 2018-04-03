package main

import (
	"fmt"
	"os"

	"gitlab.com/pions/pion/pkg/go/log"
	"gitlab.com/pions/pion/turn/server"
)

func main() {
	if os.Getenv("FQDN") == "" {
		log.Fatal().Msg("FQDN is a required environment variable")
	}

	s := server.NewTurnServer()
	errors := make(chan error)

	go func() {
		err := s.Listen("", server.DefaultPort)
		errors <- err
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
