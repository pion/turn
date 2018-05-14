package turn

import (
	"fmt"

	"github.com/pions/pkg/stun"
	"github.com/pions/turn/internal/server"
)

// Server is the interface for Pion TURN server callbacks
type Server interface {
	AuthenticateRequest(username string, srcAddr *stun.TransportAddr) (password string, ok bool)
}

// StartArguments are the arguments for the Pion TURN server
type StartArguments struct {
	Server  Server
	Realm   string
	UDPPort int
}

// Start the Pion TURN server
func Start(args StartArguments) {
	fmt.Println(server.NewServer(args.Realm, args.Server.AuthenticateRequest).Listen("", args.UDPPort))
}
