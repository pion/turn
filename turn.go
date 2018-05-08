package turn

import (
	"fmt"

	"github.com/pions/pkg/stun"
	"github.com/pions/turn/internal/turn"
)

type Server interface {
	AuthenticateRequest(username string, srcAddr *stun.TransportAddr) (password string, ok bool)
}

type StartArguments struct {
	Server Server
	Realm string
	UdpPort int
}

func Start(args StartArguments) {
	errors := make(chan error)

	go func() {
		errors <- turnServer.NewServer(args.Realm, args.Server.AuthenticateRequest).Listen("", args.UdpPort)
	}()

	select {
	case e := <-errors:
		fmt.Println(e)
	}
}
