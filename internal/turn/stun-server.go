package turnServer

import (
	"fmt"
	"net"

	"log"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// IANA assigned ports for "stun" protocol.
const (
	DefaultPort    = 3478
	DefaultTLSPort = 5349
)

type StunHandler func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error

type HandlerKey struct {
	Class  stun.MessageClass
	Method stun.Method
}

type Server struct {
	connection *ipv4.PacketConn
	packet     []byte
	handlers   map[HandlerKey]StunHandler
	realm      string
}

func NewServer(realm string) *Server {
	const (
		maxStunMessageSize = 1500
	)

	s := &Server{}
	s.packet = make([]byte, maxStunMessageSize)
	s.handlers = make(map[HandlerKey]StunHandler)
	s.realm = realm

	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodBinding}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleBindingRequest(srcAddr, dstAddr, m)
	}
	addTurnHandlers(s)

	return s
}

func (s *Server) handleBindingRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	return stun.BuildAndSend(s.connection, srcAddr, stun.ClassSuccessResponse, stun.MethodBinding, m.TransactionID,
		&stun.XorMappedAddress{
			XorAddress: stun.XorAddress{
				IP:   srcAddr.IP,
				Port: srcAddr.Port,
			},
		},
		&stun.Fingerprint{},
	)
}

func (s *Server) handleUDPPacket(dstPort int) error {
	size, cm, addr, err := s.connection.ReadFrom(s.packet)
	if err != nil {
		return errors.Wrap(err, "failed to read packet from udp socket")
	}

	dstAddr := &stun.TransportAddr{IP: cm.Dst, Port: dstPort}
	srcAddr, err := stun.NewTransportAddr(addr)
	if err != nil {
		return errors.Wrap(err, "failed reading udp addr")
	}
	packetType, err := stun.GetPacketType(s.packet[:size])
	if err != nil {
		return err
	}

	if packetType == stun.PacketTypeSTUN {
		m, err := stun.NewMessage(s.packet[:size])
		if err != nil {
			return errors.Wrap(err, "Failed to create stun message from packet")
		}

		if v, ok := s.handlers[HandlerKey{m.Class, m.Method}]; ok {
			if err := v(srcAddr, dstAddr, m); err != nil {
				log.Printf("unable to handle %v-%v from %v: %v", m.Method, m.Class, addr, err)
			}
		}
	} else if packetType == stun.PacketTypeChannelData {
		c, err := stun.NewChannelData(s.packet[:size])
		if err != nil {
			return errors.Wrap(err, "Failed to create channel data from packet")
		}
		if err := s.handleChannelData(srcAddr, dstAddr, c); err != nil {
			log.Printf("unable to handle ChannelData from %v: %v", addr, err)
		}
	}

	return nil
}

func (s *Server) Listen(address string, port int) error {
	c, err := net.ListenPacket("udp4", fmt.Sprintf("%s:%d", address, port))
	if err != nil {
		return err
	}
	s.connection = ipv4.NewPacketConn(c)
	if err := s.connection.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return err
	}

	for {
		if err := s.handleUDPPacket(port); err != nil {
			fmt.Println(err)
		}
	}
}
