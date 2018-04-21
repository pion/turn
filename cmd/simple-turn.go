package main

import (
	"github.com/pions/turn"
)

type MyTurnServer struct {
}

func main() {
	turn.Start(&MyTurnServer{})
}
