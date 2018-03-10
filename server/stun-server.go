package server

import (
	"fmt"
	"net"

	"log"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"gitlab.com/pions/pion/turn/stun"
)

type StunServer struct {
	connection *net.UDPConn
	packet     []byte
}

func NewStunServer() *StunServer {
	const (
		maxStunMessageSize = 1500
	)

	s := StunServer{}
	s.packet = make([]byte, maxStunMessageSize)

	return &s
}

func (s *StunServer) handleUDPPacket() error {

	log.Println("Waiting for packet...")
	size, addr, err := s.connection.ReadFromUDP(s.packet)
	if err != nil {
		return errors.Wrap(err, "failed to read packet from udp socket")
	}

	m, err := stun.NewMessage(s.packet[:size])
	if err != nil {
		log.Printf("failed to create stun message from packet: %v", err)
	}

	log.Printf("received message from %v of size %d, %s", addr, size, spew.Sdump(m))

	return nil
}

func (s *StunServer) Listen(address string, port int) error {
	udpAddress, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return err
	}

	log.Println("Listening...")
	conn, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return err
	}

	s.connection = conn

	for {
		err := s.handleUDPPacket()
		if err != nil {
			return errors.Wrap(err, "error handling udp packet")
		}
	}
}
