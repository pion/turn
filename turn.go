package turn

import (
	"fmt"

	"github.com/pions/pkg/stun"
	"github.com/pions/turn/internal/turn"
)

type Server interface {
	AuthenticateRequest(username string, srcAddr *stun.TransportAddr) (password string, ok bool)
}

func Start(s Server, realm string) {
	errors := make(chan error)

	go func() {
		errors <- turnServer.NewServer(realm, s.AuthenticateRequest).Listen("", turnServer.DefaultPort)
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
