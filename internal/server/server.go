package server

import (
	"bytes"
	"crypto/md5" // #nosec
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/pion/stun"
	"github.com/pion/turn/internal/allocation"
	"github.com/pion/turn/internal/ipnet"
	"github.com/pkg/errors"
)

// AuthHandler is a callback used to handle incoming auth requests, allowing users to customize Pion TURN
// with custom behavior
type AuthHandler func(username string, srcAddr *stun.TransportAddr) (password string, ok bool)

// Server is an instance of the Pion TURN server
type Server struct {
	connection         ipnet.PacketConn
	packet             []byte
	realm              string
	authHandler        AuthHandler
	manager            *allocation.Manager
	reservationManager *allocation.ReservationManager
}

// NewServer creates the Pion TURN server
func NewServer(realm string, a AuthHandler) *Server {
	const maxStunMessageSize = 1500
	return &Server{
		packet:             make([]byte, maxStunMessageSize),
		realm:              realm,
		authHandler:        a,
		manager:            &allocation.Manager{},
		reservationManager: &allocation.ReservationManager{},
	}
}

// Listen starts listening and handling TURN traffic
func (s *Server) Listen(address string, port int) error {
	listeningAddress := fmt.Sprintf("%s:%d", address, port)
	network := "udp4"
	c, err := net.ListenPacket(network, listeningAddress)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to listen on %s", listeningAddress))
	}
	conn, err := ipnet.NewPacketConn(network, c)
	if err != nil {
		return errors.Wrap(err, "failed to create connection")
	}
	s.connection = conn

	for {
		size, cm, addr, err := s.connection.ReadFromCM(s.packet)
		if err != nil {
			return errors.Wrap(err, "failed to read packet from udp socket")
		}

		srcAddr, err := stun.NewTransportAddr(addr)
		if err != nil {
			return errors.Wrap(err, "failed reading udp addr")
		}
		if err := s.handleUDPPacket(srcAddr, &stun.TransportAddr{IP: cm.Dst, Port: port}, size); err != nil {
			log.Println(err)
		}
	}
}

func (s *Server) handleUDPPacket(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, size int) error {
	packetType, err := stun.GetPacketType(s.packet[:size])
	if err != nil {
		return err
	}

	switch packetType {
	case stun.PacketTypeChannelData:
		return s.handleDataPacket(srcAddr, dstAddr, size)
	default:
		return s.handleTURNPacket(srcAddr, dstAddr, size)
	}
}

func (s *Server) handleDataPacket(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, size int) error {
	c, err := stun.NewChannelData(s.packet[:size])
	if err != nil {
		return errors.Wrap(err, "Failed to create channel data from packet")
	}

	err = s.handleChannelData(srcAddr, dstAddr, c)
	if err != nil {
		return errors.Errorf("unable to handle ChannelData from %v: %v", srcAddr, err)
	}

	return nil
}

func (s *Server) handleTURNPacket(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, size int) error {
	m, err := stun.NewMessage(s.packet[:size])
	if err != nil {
		return errors.Wrap(err, "failed to create stun message from packet")
	}

	h, err := s.getMessageHandler(m.Class, m.Method)
	if err != nil {
		return errors.Errorf("unhandled STUN packet %v-%v from %v: %v", m.Method, m.Class, srcAddr, err)
	}

	err = h(srcAddr, dstAddr, m)
	if err != nil {
		return errors.Errorf("failed to handle %v-%v from %v: %v", m.Method, m.Class, srcAddr, err)
	}

	return nil
}

type messageHandler func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error

func (s *Server) getMessageHandler(class stun.MessageClass, method stun.Method) (messageHandler, error) {
	switch class {
	case stun.ClassIndication:
		switch method {
		case stun.MethodSend:
			return s.handleSendIndication, nil
		default:
			return nil, errors.Errorf("unexpected method: %s", method)
		}

	case stun.ClassRequest:
		switch method {
		case stun.MethodAllocate:
			return s.handleAllocateRequest, nil
		case stun.MethodRefresh:
			return s.handleRefreshRequest, nil
		case stun.MethodCreatePermission:
			return s.handleCreatePermissionRequest, nil
		case stun.MethodChannelBind:
			return s.handleChannelBindRequest, nil
		case stun.MethodBinding:
			return s.handleBindingRequest, nil
		default:
			return nil, errors.Errorf("unexpected method: %s", method)
		}

	default:
		return nil, errors.Errorf("unexpected class: %s", class)
	}
}

// Is there really no stdlib for this?
func min(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func randSeq(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// TODO, include time info support stale nonces
func buildNonce() string {
	/* #nosec */
	h := md5.New()
	now := time.Now().Unix()
	if _, err := io.WriteString(h, strconv.FormatInt(now, 10)); err != nil {
		fmt.Printf("Failed generating nonce %v \n", err)
	}
	if _, err := io.WriteString(h, strconv.FormatInt(rand.Int63(), 10)); err != nil {
		fmt.Printf("Failed generating nonce %v \n", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func assertMessageIntegrity(m *stun.Message, theirMi *stun.RawAttribute, ourKey []byte) error {
	// Length to remove when comparing MessageIntegrity (so we can re-compute)
	tailLength := 24
	rawCopy := make([]byte, len(m.Raw))
	copy(rawCopy, m.Raw)

	if _, messageIntegrityAttrFound := m.GetOneAttribute(stun.AttrFingerprint); messageIntegrityAttrFound {
		currLength := binary.BigEndian.Uint16(rawCopy[2:4])

		binary.BigEndian.PutUint16(rawCopy[2:], currLength-8)
		tailLength += 8
	}

	ourMi, err := stun.MessageIntegrityCalculateHMAC(ourKey, rawCopy[:len(rawCopy)-tailLength])
	if err != nil {
		return err
	}

	if !bytes.Equal(ourMi, theirMi.Value) {
		return errors.Errorf("MessageIntegrity mismatch %x %x", ourKey, theirMi.Value)
	}
	return nil
}
