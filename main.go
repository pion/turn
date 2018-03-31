package main

import (
	"fmt"

	"gitlab.com/pions/pion/turn/server"
)

func main() {
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
