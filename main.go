package main

import (
	"fmt"
	"os"

	"gitlab.com/pions/pion/pkg/go/log"
	"gitlab.com/pions/pion/turn/internal/turn"
)

func main() {
	if os.Getenv("FQDN") == "" {
		log.Fatal().Msg("FQDN is a required environment variable")
	}

	s := turnServer.NewServer()
	errors := make(chan error)

	go func() {
		err := s.Listen("", turnServer.DefaultPort)
		errors <- err
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
