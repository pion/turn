package server

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/pions/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// AuthHandler is a callback used to handle incoming auth requests, allowing users to customize Pion TURN
// with custom behavior
type AuthHandler func(username string, srcAddr *stun.TransportAddr) (password string, ok bool)

// Server is an instance of the Pion TURN server
type Server struct {
	connection  *ipv4.PacketConn
	packet      []byte
	realm       string
	authHandler AuthHandler
}

// NewServer creates the Pion TURN server
func NewServer(realm string, a AuthHandler) *Server {
	const maxStunMessageSize = 1500
	return &Server{
		packet:      make([]byte, maxStunMessageSize),
		realm:       realm,
		authHandler: a,
	}
}

// Listen starts listening and handling TURN traffic
func (s *Server) Listen(address string, port int) error {

	listeningAddress := fmt.Sprintf("%s:%d", address, port)
	c, err := net.ListenPacket("udp4", listeningAddress)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to listen on %s", listeningAddress))
	}
	s.connection = ipv4.NewPacketConn(c)
	if err := s.connection.SetControlMessage(ipv4.FlagDst, true); err != nil {
		return errors.Wrap(err, "Failed to SetControlMessage ipv4.FlagDst")
	}

	for {
		size, cm, addr, err := s.connection.ReadFrom(s.packet)
		if err != nil {
			return errors.Wrap(err, "failed to read packet from udp socket")
		}

		srcAddr, err := stun.NewTransportAddr(addr)
		if err != nil {
			return errors.Wrap(err, "failed reading udp addr")
		}
		if err := s.handleUDPPacket(srcAddr, &stun.TransportAddr{IP: cm.Dst, Port: port}, s.packet, size); err != nil {
			log.Println(err)
		}
	}
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
			return errors.Errorf("Failed to handle %v-%v from %v: %v", m.Method, m.Class, srcAddr, err)
		}
		return nil
	}

	return errors.Errorf("Unhandled STUN packet %v-%v from %v", m.Method, m.Class, srcAddr)
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

	ourMi, err := stun.MessageIntegrityCalculateHMAC(ourKey[:], rawCopy[:len(rawCopy)-tailLength])
	if err != nil {
		return err
	}

	if !bytes.Equal(ourMi, theirMi.Value) {
		return errors.Errorf("MessageIntegrity mismatch %x %x", ourKey, theirMi.Value)
	}
	return nil
}
