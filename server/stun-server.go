package server

import (
	"fmt"
	"net"

	"log"

	"github.com/pkg/errors"
	"gitlab.com/pions/pion/pkg/go/stun"
)

// IANA assigned ports for "stun" protocol.
const (
	DefaultPort    = 3478
	DefaultTLSPort = 5349
)

type StunHandler func(addr *net.UDPAddr, m *stun.Message) error

type HandlerKey struct {
	Class  stun.MessageClass
	Method stun.Method
}

type StunServer struct {
	connection *net.UDPConn
	packet     []byte
	handlers   map[HandlerKey]StunHandler
}

func NewStunServer() *StunServer {
	const (
		maxStunMessageSize = 1500
	)

	s := StunServer{}
	s.packet = make([]byte, maxStunMessageSize)
	s.handlers = make(map[HandlerKey]StunHandler)

	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodBinding}] = func(addr *net.UDPAddr, m *stun.Message) error {
		return s.handleBindingRequest(addr, m)
	}

	return &s
}

func (s *StunServer) handleBindingRequest(addr *net.UDPAddr, m *stun.Message) error {
	return buildAndSend(s.connection, addr, stun.ClassSuccessResponse, stun.MethodBinding, m.TransactionID,
		&stun.XorMappedAddress{
			IP:   addr.IP,
			Port: addr.Port,
		},
		&stun.Fingerprint{},
	)
	// log.Printf("received message from %v, %s", addr, spew.Sdump(m))
	// log.Printf("response message to %v of size %d, %s", addr, rsp.Length, spew.Sdump(rsp))
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

	if v, ok := s.handlers[HandlerKey{m.Class, m.Method}]; ok {
		if err := v(addr, m); err != nil {
			log.Printf("unable to handle %v-%v from %v: %v", m.Method, m.Class, addr, err)
		}
	}

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

	// Only fatal errors should bubble out to here
	for {
		err = s.handleUDPPacket()
		if err != nil {
			return errors.Wrap(err, "error handling udp packet")
		}
	}
}
