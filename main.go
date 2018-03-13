package main

import (
	"fmt"

	"gitlab.com/pions/pion/turn/server"
	"gitlab.com/pions/pion/turn/stun"
)

func main() {
	s := server.NewStunServer()
	errors := make(chan error)

	go func() {
		err := s.Listen("", stun.DefaultPort)
		errors <- err
	}()

	for {
		select {
		case e := <-errors:
			fmt.Println(e)
		}
	}
}
