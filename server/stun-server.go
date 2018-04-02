package server

import (
	"fmt"
	"net"

	"log"

	"github.com/pkg/errors"
	"gitlab.com/pions/pion/pkg/go/stun"
	"golang.org/x/net/ipv4"
)

// IANA assigned ports for "stun" protocol.
const (
	DefaultPort    = 3478
	DefaultTLSPort = 5349
)

type StunHandler func(srcAddr net.Addr, dstIp net.IP, m *stun.Message) error

type HandlerKey struct {
	Class  stun.MessageClass
	Method stun.Method
}

type StunServer struct {
	connection *ipv4.PacketConn
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

	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodBinding}] = func(srcAddr net.Addr, dstIp net.IP, m *stun.Message) error {
		return s.handleBindingRequest(srcAddr, dstIp, m)
	}

	return &s
}

func (s *StunServer) handleBindingRequest(srcAddr net.Addr, dstIp net.IP, m *stun.Message) error {
	ip, port, err := netAddrIPPort(srcAddr)
	if err != nil {
		return errors.Wrap(err, "Failed to take net.Addr to Host/Port")
	}

	return buildAndSend(s.connection, srcAddr, stun.ClassSuccessResponse, stun.MethodBinding, m.TransactionID,
		&stun.XorMappedAddress{
			stun.XorAddress{
				IP:   ip,
				Port: port,
			},
		},
		&stun.Fingerprint{},
	)
}

func (s *StunServer) handleUDPPacket() error {
	size, cm, addr, err := s.connection.ReadFrom(s.packet)
	if err != nil {
		return errors.Wrap(err, "failed to read packet from udp socket")
	}

	if _, err := s.connection.WriteTo([]byte{'A', 'B', 'C'}, nil, addr); err != nil {
		log.Fatal(err)
	}

	m, err := stun.NewMessage(s.packet[:size])
	if err != nil {
		log.Printf("failed to create stun message from packet: %v", err)
	}

	if v, ok := s.handlers[HandlerKey{m.Class, m.Method}]; ok {
		if err := v(addr, cm.Dst, m); err != nil {
			log.Printf("unable to handle %v-%v from %v: %v", m.Method, m.Class, addr, err)
		}
	}

	return nil
}

func (s *StunServer) Listen(address string, port int) error {
	c, err := net.ListenPacket("udp4", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return err
	}
	s.connection = ipv4.NewPacketConn(c)
	if err := s.connection.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return err
	}

	for {
		if err := s.handleUDPPacket(); err != nil {
			return err
		}
	}
}
