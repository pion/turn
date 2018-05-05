package relayServer

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// Public
type Permission struct {
	IP           net.IP
	Port         int
	TimeToExpiry uint32
}

type Protocol int

const (
	UDP Protocol = iota
	TCP Protocol = iota
)

type FiveTuple struct {
	SrcAddr  *stun.TransportAddr
	DstAddr  *stun.TransportAddr
	Protocol Protocol
}

func (a *FiveTuple) match(b *FiveTuple) bool {
	return a.SrcAddr.Equal(b.SrcAddr) &&
		a.DstAddr.Equal(b.DstAddr) &&
		a.Protocol == b.Protocol
}

type ChannelBind struct {
	Peer *stun.TransportAddr
	// expiration uint32
}

func Start(turnSocket *ipv4.PacketConn, fiveTuple *FiveTuple, reservationToken string, lifetime uint32, username string) (listeningPort int, err error) {
	s := &server{
		FiveTuple:        fiveTuple,
		reservationToken: reservationToken,
		lifetime:         lifetime,
		channelBindings:  map[uint16]*ChannelBind{},
		turnSocket:       turnSocket,
	}

	listener, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}

	s.relaySocket = ipv4.NewPacketConn(listener)
	err = s.relaySocket.SetControlMessage(ipv4.FlagDst, true)
	if err != nil {
		return
	}

	listeningAddr, err := stun.NewTransportAddr(listener.LocalAddr())
	if err != nil {
		return
	}

	listeningPort = listeningAddr.Port
	s.listeningPort = listeningPort
	s.username = username

	serversLock.Lock()
	servers = append(servers, s)
	serversLock.Unlock()

	go relayHandler(s)
	return
}

//Caller must unlock mutex
func getServer(fiveTuple *FiveTuple) (server *server) {
	serversLock.RLock()

	for _, s := range servers {
		if fiveTuple.match(s.FiveTuple) {
			server = s
		}
	}
	return
}

func Fulfilled(fiveTuple *FiveTuple) bool {
	defer serversLock.RUnlock()
	return getServer(fiveTuple) != nil
}

func AddPermission(fiveTuple *FiveTuple, permission *Permission) error {
	s := getServer(fiveTuple)
	serversLock.RUnlock()
	if s == nil {
		return errors.Errorf("Unable to add permission, server not found")
	}
	s.permissionsLock.Lock()
	defer s.permissionsLock.Unlock()
	for _, p := range s.permissions {
		if p.Port == permission.Port && p.IP.Equal(permission.IP) {
			return nil
		}
	}

	s.permissions = append(s.permissions, permission)
	return nil
}

func GetRelayForSrc(addr *stun.TransportAddr) (*ipv4.PacketConn, error) {
	serversLock.RLock()
	defer serversLock.RUnlock()

	for _, s := range servers {
		if s.FiveTuple.SrcAddr.Equal(addr) {
			return s.relaySocket, nil
		}
	}

	return nil, errors.Errorf("No Relay is allocated to this src %d", addr.Port)
}

func GetChannelById(id uint16, fiveTuple *FiveTuple) *ChannelBind {
	s := getServer(fiveTuple)
	serversLock.RUnlock()

	if s == nil {
		return nil
	}

	s.channelBindingsLock.RLock()
	defer s.channelBindingsLock.RUnlock()

	cb, _ := s.channelBindings[id]
	return cb
}

func GetChannelByPeer(peer *stun.TransportAddr, fiveTuple *FiveTuple) *ChannelBind {
	s := getServer(fiveTuple)
	serversLock.RUnlock()

	if s == nil {
		return nil
	}

	s.channelBindingsLock.RLock()
	defer s.channelBindingsLock.RUnlock()
	for _, cb := range s.channelBindings {
		if cb.Peer.Equal(peer) {
			return cb
		}
	}
	return nil
}

func AddChannelBind(peer *stun.TransportAddr, id uint16, fiveTuple *FiveTuple) error {
	server := getServer(fiveTuple)
	serversLock.RUnlock()

	if server == nil {
		return errors.Errorf("Failed to AddChannelBind, no server found for FiveTuple: %x", fiveTuple)
	}

	// Check that this channel id isn't bound to another transport address, and
	// that this transport address isn't bound to another channel id.
	byId := GetChannelById(id, fiveTuple)
	if byId != nil {
		return errors.Errorf("Failed to AddChannelBind, ChannelNumber taken: %x", id)
	}

	byPeer := GetChannelByPeer(peer, fiveTuple)
	if byPeer != nil {
		return errors.Errorf("Failed to AddChannelBind, Peer taken: %v", peer)
	}

	server.channelBindingsLock.Lock()
	server.channelBindings[id] = &ChannelBind{Peer: peer}
	server.channelBindingsLock.Unlock()

	return nil
}

// Private
type server struct {
	*FiveTuple
	listeningPort              int
	reservationToken, username string
	lifetime                   uint32

	permissionsLock sync.RWMutex
	permissions     []*Permission

	channelBindingsLock sync.RWMutex
	channelBindings     map[uint16]*ChannelBind

	turnSocket  *ipv4.PacketConn
	relaySocket *ipv4.PacketConn
}

var serversLock sync.RWMutex
var servers []*server

const RtpMTU = 1500

//  https://tools.ietf.org/html/rfc5766#section-10.3
//  When the server receives a UDP datagram at a currently allocated
//  relayed transport address, the server looks up the allocation
//  associated with the relayed transport address.  The server then
//  checks to see whether the set of permissions for the allocation allow
//  the relaying of the UDP datagram as described in Section 8.
//
//  If relaying is permitted, then the server checks if there is a
//  channel bound to the peer that sent the UDP datagram (see
//  Section 11).  If a channel is bound, then processing proceeds as
//  described in Section 11.7.
//
//  If relaying is permitted but no channel is bound to the peer, then
//  the server forms and sends a Data indication.  The Data indication
//  MUST contain both an XOR-PEER-ADDRESS and a DATA attribute.  The DATA
//  attribute is set to the value of the 'data octets' field from the
//  datagram, and the XOR-PEER-ADDRESS attribute is set to the source
//  transport address of the received UDP datagram.  The Data indication
//  is then sent on the 5-tuple associated with the allocation.
func relayHandler(s *server) {
	buffer := make([]byte, RtpMTU)

	for {
		n, _, _ /*srcAddr*/, err := s.relaySocket.ReadFrom(buffer)
		if err != nil {
			fmt.Println("Failing to relay")
		}

		channelData := make([]byte, 4)
		binary.BigEndian.PutUint16(channelData[0:], uint16(0x4000))
		binary.BigEndian.PutUint16(channelData[2:], uint16(n))
		channelData = append(channelData, buffer[:n]...)

		s.turnSocket.WriteTo(channelData, nil, s.FiveTuple.SrcAddr.Addr())

		// dataAttr := stun.Data{Data: buffer[:n]}
		// xorPeerAddressAttr := stun.XorPeerAddress{stun.XorAddress{IP: srcAddr.(*net.UDPAddr).IP, Port: srcAddr.(*net.UDPAddr).Port}}

		// _ = stun.BuildAndSend(s.turnSocket, s.FiveTuple.SrcAddr, stun.ClassIndication, stun.MethodData, buildTransactionId(), &xorPeerAddressAttr, &dataAttr)
	}
}
