package turn

import (
	"fmt"
	"os"

	"github.com/pions/turn/internal/turn"
	"github.com/rs/zerolog/log"
)

type Server interface {
}

func Start(s Server) {
	if os.Getenv("FQDN") == "" {
		log.Fatal().Msg("FQDN is a required environment variable")
	}

	tServer := turnServer.NewServer()
	errors := make(chan error)

	go func() {
		err := tServer.Listen("", turnServer.DefaultPort)
		errors <- err
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
