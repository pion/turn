package server

import (
	"crypto/md5" // #nosec
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/stun"
	"github.com/pion/turn/internal/allocation"
	"github.com/pion/turn/internal/ipnet"
	"github.com/pkg/errors"
)

// AuthHandler is a callback used to handle incoming auth requests, allowing users to customize Pion TURN
// with custom behavior
type AuthHandler func(username string, srcAddr net.Addr) (password string, ok bool)

// Server is an instance of the Pion TURN server
type Server struct {
	lock               sync.RWMutex
	connection         ipnet.PacketConn
	packet             []byte
	realm              string
	authHandler        AuthHandler
	manager            *allocation.Manager
	reservationManager *allocation.ReservationManager
	channelBindTimeout time.Duration
}

// NewServer creates the Pion TURN server
func NewServer(realm string, channelBindTimeout time.Duration, a AuthHandler) *Server {
	const maxStunMessageSize = 1500
	return &Server{
		packet:             make([]byte, maxStunMessageSize),
		realm:              realm,
		authHandler:        a,
		manager:            &allocation.Manager{},
		reservationManager: &allocation.ReservationManager{},
		channelBindTimeout: channelBindTimeout,
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
	s.lock.Lock()
	s.connection = conn
	s.lock.Unlock()

	for {
		size, cm, addr, err := s.connection.ReadFromCM(s.packet)
		if err != nil {
			return errors.Wrap(err, "failed to read packet from udp socket")
		}

		if err := s.handleUDPPacket(addr, &net.UDPAddr{IP: cm.Dst, Port: port}, size); err != nil {
			log.Println(err)
		}
	}
}

// Close closes the connection.
func (s *Server) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.manager.Close(); err != nil {
		return err
	}
	return s.connection.Close()
}

func (s *Server) handleUDPPacket(srcAddr, dstAddr net.Addr, size int) error {
	if turn.IsChannelData(s.packet[:size]) {
		return s.handleDataPacket(srcAddr, dstAddr, size)
	}

	return s.handleTURNPacket(srcAddr, dstAddr, size)
}

func (s *Server) handleDataPacket(srcAddr, dstAddr net.Addr, size int) error {
	c := turn.ChannelData{Raw: s.packet[:size]}
	if err := c.Decode(); err != nil {
		return errors.Wrap(err, "Failed to create channel data from packet")
	}

	err := s.handleChannelData(srcAddr, dstAddr, &c)
	if err != nil {
		err = errors.Errorf("unable to handle ChannelData from %v: %v", srcAddr, err)
	}

	return err
}

func (s *Server) handleTURNPacket(srcAddr, dstAddr net.Addr, size int) error {
	m := &stun.Message{Raw: append([]byte{}, s.packet[:size]...)}
	if err := m.Decode(); err != nil {
		return errors.Wrap(err, "failed to create stun message from packet")
	}

	h, err := s.getMessageHandler(m.Type.Class, m.Type.Method)
	if err != nil {
		return errors.Errorf("unhandled STUN packet %v-%v from %v: %v", m.Type.Method, m.Type.Class, srcAddr, err)
	}

	err = h(srcAddr, dstAddr, m)
	if err != nil {
		return errors.Errorf("failed to handle %v-%v from %v: %v", m.Type.Method, m.Type.Class, srcAddr, err)
	}

	return nil
}

type messageHandler func(srcAddr, dstAddr net.Addr, m *stun.Message) error

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

func assertMessageIntegrity(m *stun.Message, ourKey []byte) error {
	messageIntegrityAttr := stun.MessageIntegrity(ourKey)
	return messageIntegrityAttr.Check(m)
}

func buildAndSend(conn net.PacketConn, dst net.Addr, attrs ...stun.Setter) error {
	msg, err := stun.Build(attrs...)
	if err != nil {
		return err
	}

	_, err = conn.WriteTo(msg.Raw, dst)
	return err
}
