package turn

import (
	"fmt"

	"github.com/pions/turn/internal/turn"
)

type Server interface {
}

func Start(s Server, realm string) {
	errors := make(chan error)

	go func() {
		errors <- turnServer.NewServer(realm).Listen("", turnServer.DefaultPort)
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
