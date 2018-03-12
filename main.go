package main

import "gitlab.com/pions/pion/turn/stun"
import (
	"gitlab.com/pions/pion/turn/server"
)

func main() {
	s := server.NewStunServer()
	errors := make(chan error)

	go func() {
		err := s.Listen("", stun.DefaultPort)
		errors <- err
	}()

	select {
	case e := <-errors:
		panic(e)
	}
}
