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

type AuthHandler func(username string, srcAddr *stun.TransportAddr) (password string, ok bool)
type Server struct {
	connection  *ipv4.PacketConn
	packet      []byte
	realm       string
	authHandler AuthHandler
}

func NewServer(realm string, a AuthHandler) *Server {
	const maxStunMessageSize = 1500
	return &Server{
		packet:      make([]byte, maxStunMessageSize),
		realm:       realm,
		authHandler: a,
	}
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

func (s *Server) handleUDPPacket(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, packet []byte, size int) error {
	packetType, err := stun.GetPacketType(s.packet[:size])
	if err != nil {
		return err
	} else if packetType == stun.PacketTypeChannelData {
		c, err := stun.NewChannelData(s.packet[:size])
		if err != nil {
			return errors.Wrap(err, "Failed to create channel data from packet")
		}
		if err := s.handleChannelData(srcAddr, dstAddr, c); err != nil {
			return errors.Errorf("unable to handle ChannelData from %v: %v", srcAddr, err)
		}
		return nil
	}

	m, err := stun.NewMessage(s.packet[:size])
	if err != nil {
		return errors.Wrap(err, "Failed to create stun message from packet")
	}

	if m.Class == stun.ClassIndication && m.Method == stun.MethodSend {
		if err := s.handleSendIndication(srcAddr, dstAddr, m); err != nil {
			return errors.Errorf("unable to handle %v-%v from %v: %v", m.Method, m.Class, srcAddr, err)
		}
		return nil
	} else if m.Class == stun.ClassRequest {
		switch m.Method {
		case stun.MethodAllocate:
			err = s.handleAllocateRequest(srcAddr, dstAddr, m)
		case stun.MethodRefresh:
			err = s.handleRefreshRequest(srcAddr, dstAddr, m)
		case stun.MethodCreatePermission:
			err = s.handleCreatePermissionRequest(srcAddr, dstAddr, m)
		case stun.MethodChannelBind:
			err = s.handleChannelBindRequest(srcAddr, dstAddr, m)
		case stun.MethodBinding:
			err = s.handleBindingRequest(srcAddr, dstAddr, m)
		}
		if err != nil {
			return errors.Errorf("unable to handle %v-%v from %v: %v", m.Method, m.Class, srcAddr, err)
		}
		return nil
	}

	return errors.Errorf("Unhandled packet from %v", srcAddr)
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
		size, cm, addr, err := s.connection.ReadFrom(s.packet)
		if err != nil {
			return errors.Wrap(err, "failed to read packet from udp socket")
		}

		dstAddr := &stun.TransportAddr{IP: cm.Dst, Port: port}
		srcAddr, err := stun.NewTransportAddr(addr)
		if err != nil {
			return errors.Wrap(err, "failed reading udp addr")
		}
		if err := s.handleUDPPacket(srcAddr, dstAddr, s.packet, size); err != nil {
			log.Println(err)
		}
	}
}
