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

	rsp := &stun.Message{
		Method:        stun.MethodBinding,
		Class:         stun.ClassSuccessResponse,
		TransactionID: m.TransactionID,
	}

	xorAddr := stun.XorMappedAddress{
		IP:   addr.IP,
		Port: addr.Port,
	}

	ra, err := xorAddr.Pack(rsp)
	if err != nil {
		return errors.Wrap(err, "unable to pack XOR-MAPPED-ADDRESS attribute")
	}

	rsp.Attributes = append(rsp.Attributes, ra)

	b := rsp.Pack()

	m2, err := stun.NewMessage(b)
	if err != nil {
		return errors.Wrap(err, "invalid response generated")
	}

	err = xorAddr.Unpack(m2, m2.Attributes[0])
	if err != nil {
		return errors.Wrap(err, "unable to decode response xoraddr")
	}

	log.Printf("Return XOR: %s", spew.Sdump(xorAddr))

	l, err := s.connection.WriteTo(b, addr)
	if err != nil {
		return errors.Wrap(err, "failed writing to socket")
	}
	if l != len(b) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected)", l, len(b))
	}

	log.Printf("received message from %v of size %d, %s", addr, size, spew.Sdump(m))
	log.Printf("response message to %v of size %d, %s", addr, rsp.Length, spew.Sdump(rsp))

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
