package turn

import (
	"crypto/md5" // #nosec
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/transport/vnet"
	"github.com/pion/turn/internal/allocation"
	"github.com/pkg/errors"
)

const (
	maxStunMessageSize = 1500
)

// AuthHandler is a callback used to handle incoming auth requests, allowing users to customize Pion TURN
// with custom behavior
type AuthHandler func(username string, srcAddr net.Addr) (password string, ok bool)

// ServerConfig is a bag of config parameters for Server.
type ServerConfig struct {
	Realm              string
	AuthHandler        AuthHandler
	ChannelBindTimeout time.Duration
	ListeningPort      int
	LoggerFactory      logging.LoggerFactory
	Net                *vnet.Net
}

type listener struct {
	conn    net.PacketConn
	closeCh chan struct{}
}

// Server is an instance of the Pion TURN server
type Server struct {
	lock               sync.RWMutex
	listeners          []*listener
	listenIPs          []net.IP
	listenPort         int
	realm              string
	authHandler        AuthHandler
	manager            *allocation.Manager
	reservationManager *allocation.ReservationManager
	channelBindTimeout time.Duration
	log                logging.LeveledLogger
	net                *vnet.Net
}

// NewServer creates the Pion TURN server
func NewServer(config *ServerConfig) *Server {
	log := config.LoggerFactory.NewLogger("turn")

	if config.Net == nil {
		config.Net = vnet.NewNet(nil) // defaults to native operation
	} else {
		log.Warn("vnet is enabled")
	}

	manager := allocation.NewManager(&allocation.ManagerConfig{
		LeveledLogger: log,
		Net:           config.Net,
	})

	listenPort := config.ListeningPort
	if listenPort == 0 {
		listenPort = 3478
	}

	return &Server{
		listenPort:         listenPort,
		realm:              config.Realm,
		authHandler:        config.AuthHandler,
		manager:            manager,
		reservationManager: &allocation.ReservationManager{},
		channelBindTimeout: config.ChannelBindTimeout,
		log:                log,
		net:                config.Net,
	}
}

// AddListeningIPAddr adds a listening IP address.
// This method must be called before calling Start().
func (s *Server) AddListeningIPAddr(addrStr string) error {
	ip := net.ParseIP(addrStr)
	if ip.To4() == nil {
		return fmt.Errorf("Non-IPv4 address is not supported")
	}

	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("link-local unicast address is not allowed")
	}
	s.listenIPs = append(s.listenIPs, ip)
	return nil
}

// caller must hold the mutex
func (s *Server) gatherSystemIPAddrs() error {
	s.log.Debug("gathering local IP address...")

	ifs, err := s.net.Interfaces()
	if err != nil {
		return err
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 {
			continue // skip if interface is not up
		}

		if ifc.Flags&net.FlagLoopback != 0 {
			continue // skip loopback address
		}

		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch addr := addr.(type) {
			case *net.IPNet:
				ip = addr.IP
			case *net.IPAddr:
				ip = addr.IP
			}

			if ip == nil {
				return fmt.Errorf("invalid IP address: %s", addr.String())
			}

			if ip.To4() == nil {
				continue // skip non-IPv4 address
			}

			if ip.IsLinkLocalUnicast() {
				continue
			}

			s.log.Debugf("- found local IP: %s", ip.String())
			s.listenIPs = append(s.listenIPs, ip)
		}
	}

	return nil
}

// Listen starts listening and handling TURN traffic
// caller must hold the mutex
func (s *Server) listen(localIP net.IP) error {
	network := "udp4"
	listenAddr := fmt.Sprintf("%s:%d", localIP.String(), s.listenPort)
	conn, err := s.net.ListenPacket(network, listenAddr)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to listen on %s", listenAddr))
	}

	laddr := conn.LocalAddr()
	s.log.Infof("Listening on %s:%s", laddr.Network(), laddr.String())

	closeCh := make(chan struct{})
	s.listeners = append(s.listeners, &listener{
		conn:    conn,
		closeCh: closeCh,
	})

	go func() {
		buf := make([]byte, maxStunMessageSize)
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				s.log.Debugf("exit read loop on error: %s", err.Error())
				break
			}

			if err := s.handleUDPPacket(conn, addr, buf[:n]); err != nil {
				s.log.Error(err.Error())
			}
		}

		close(closeCh)
	}()

	return nil
}

// Start starts the server.
func (s *Server) Start() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// If s.listenIPs is empty, gather system IPs
	if len(s.listenIPs) == 0 {
		err := s.gatherSystemIPAddrs()
		if err != nil {
			return err
		}

		if len(s.listenIPs) == 0 {
			return fmt.Errorf("no local IP address found")
		}
	}

	for _, localIP := range s.listenIPs {
		err := s.listen(localIP)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close closes the connection.
func (s *Server) Close() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if err := s.manager.Close(); err != nil {
		return err
	}

	for _, l := range s.listeners {
		err := l.conn.Close()
		if err != nil {
			s.log.Debugf("Close() returned error: %s", err.Error())
		}

	}

	for _, l := range s.listeners {
		<-l.closeCh
	}

	return nil
}

// caller must hold the mutex
func (s *Server) handleUDPPacket(conn net.PacketConn, srcAddr net.Addr, buf []byte) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.log.Debugf("received %d bytes of udp from %s on %s",
		len(buf),
		srcAddr.String(),
		conn.LocalAddr().String(),
	)
	if turn.IsChannelData(buf) {
		return s.handleDataPacket(conn, srcAddr, buf)
	}

	return s.handleTURNPacket(conn, srcAddr, buf)
}

// caller must hold the mutex
func (s *Server) handleDataPacket(conn net.PacketConn, srcAddr net.Addr, buf []byte) error {
	c := turn.ChannelData{Raw: buf}
	if err := c.Decode(); err != nil {
		return errors.Wrap(err, "Failed to create channel data from packet")
	}

	err := s.handleChannelData(conn, srcAddr, &c)
	if err != nil {
		err = errors.Errorf("unable to handle ChannelData from %v: %v", srcAddr, err)
	}

	return err
}

// caller must hold the mutex
func (s *Server) handleTURNPacket(conn net.PacketConn, srcAddr net.Addr, buf []byte) error {
	m := &stun.Message{Raw: append([]byte{}, buf...)}
	if err := m.Decode(); err != nil {
		return errors.Wrap(err, "failed to create stun message from packet")
	}

	h, err := s.getMessageHandler(m.Type.Class, m.Type.Method)
	if err != nil {
		return errors.Errorf("unhandled STUN packet %v-%v from %v: %v", m.Type.Method, m.Type.Class, srcAddr, err)
	}

	err = h(conn, srcAddr, m)
	if err != nil {
		return errors.Errorf("failed to handle %v-%v from %v: %v", m.Type.Method, m.Type.Class, srcAddr, err)
	}

	return nil
}

type messageHandler func(conn net.PacketConn, srcAddr net.Addr, m *stun.Message) error

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
func buildNonce() (string, error) {
	/* #nosec */
	h := md5.New()
	now := time.Now().Unix()
	if _, err := io.WriteString(h, strconv.FormatInt(now, 10)); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("Failed generating nonce %v \n", err))
	}
	if _, err := io.WriteString(h, strconv.FormatInt(rand.Int63(), 10)); err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("Failed generating nonce %v \n", err))
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
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
