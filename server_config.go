// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"crypto/md5" //nolint:gosec,gci
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4/internal/allocation"
)

// RelayAddressGenerator is used to generate a RelayAddress when creating an allocation.
// You can use one of the provided ones or provide your own.
type RelayAddressGenerator interface {
	// Validate confirms that the RelayAddressGenerator is properly initialized
	Validate() error

	// Allocate a PacketConn (UDP) RelayAddress
	AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error)

	// Allocate a Conn (TCP) RelayAddress
	AllocateConn(network string, requestedPort int) (net.Conn, net.Addr, error)
}

// PermissionHandler is a callback to filter incoming CreatePermission and ChannelBindRequest
// requests based on the client IP address and port and the peer IP address the client intends to
// connect to. If the client is behind a NAT then the filter acts on the server reflexive
// ("mapped") address instead of the real client IP address and port. Note that TURN permissions
// are per-allocation and per-peer-IP-address, to mimic the address-restricted filtering mechanism
// of NATs that comply with [RFC4787], see https://tools.ietf.org/html/rfc5766#section-2.3.
type PermissionHandler func(clientAddr net.Addr, peerIP net.IP) (ok bool)

// DefaultPermissionHandler is convince function that grants permission to all peers.
func DefaultPermissionHandler(net.Addr, net.IP) (ok bool) {
	return true
}

// PacketConnConfig is a single net.PacketConn to listen/write on.
// This will be used for UDP listeners.
type PacketConnConfig struct {
	PacketConn net.PacketConn

	// When an allocation is generated the RelayAddressGenerator
	// creates the net.PacketConn and returns the IP/Port it is available at
	RelayAddressGenerator RelayAddressGenerator

	// PermissionHandler is a callback to filter peer addresses. Can be set as nil, in which
	// case the DefaultPermissionHandler is automatically instantiated to admit all peer
	// connections
	PermissionHandler PermissionHandler
}

func (c *PacketConnConfig) validate() error {
	if c.PacketConn == nil {
		return errConnUnset
	}

	if c.RelayAddressGenerator != nil {
		if err := c.RelayAddressGenerator.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// ListenerConfig is a single net.Listener to accept connections on.
// This will be used for TCP, TLS and DTLS listeners.
type ListenerConfig struct {
	Listener net.Listener

	// When an allocation is generated the RelayAddressGenerator
	// creates the net.PacketConn and returns the IP/Port it is available at
	RelayAddressGenerator RelayAddressGenerator

	// PermissionHandler is a callback to filter peer addresses. Can be set as nil, in which
	// case the DefaultPermissionHandler is automatically instantiated to admit all peer
	// connections
	PermissionHandler PermissionHandler
}

func (c *ListenerConfig) validate() error {
	if c.Listener == nil {
		return errListenerUnset
	}

	if c.RelayAddressGenerator == nil {
		return errRelayAddressGeneratorUnset
	}

	return c.RelayAddressGenerator.Validate()
}

// AuthHandler is a callback used to handle incoming auth requests,
// allowing users to customize Pion TURN with custom behavior.
type AuthHandler func(username, realm string, srcAddr net.Addr) (key []byte, ok bool)

// GenerateAuthKey is a convenience function to easily generate keys in the format used by AuthHandler.
func GenerateAuthKey(username, realm, password string) []byte {
	// #nosec
	h := md5.New()
	fmt.Fprint(h, strings.Join([]string{username, realm, password}, ":")) // nolint: errcheck

	return h.Sum(nil)
}

// EventHandlers is a set of callbacks that the server will call at certain hook points during an
// allocation's lifecycle. All events are reported with the context that identifies the allocation
// triggering the event (source and destination address, protocol, username and realm used for
// authenticating the allocation). It is OK to handle only a subset of the callbacks.
type EventHandlers struct {
	// OnAuth is called after an authentication request has been processed with the TURN method
	// triggering the authentication request (either "Allocate", "Refresh" "CreatePermission",
	// or "ChannelBind"), and the verdict is the authentication result.
	OnAuth func(srcAddr, dstAddr net.Addr, protocol, username, realm string, method string, verdict bool)
	// OnAllocationCreated is called after a new allocation has been made. The relayAddr
	// argument specifies the relay address and requestedPort is the port requested by the
	// client (if any).
	OnAllocationCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, requestedPort int)
	// OnAllocationDeleted is called after an allocation has been removed.
	OnAllocationDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string)
	// OnAllocationError is called when the readloop hdndling an allocation exits with an
	// error with an error message.
	OnAllocationError func(srcAddr, dstAddr net.Addr, protocol, message string)
	// OnPermissionCreated is called after a new permission has been made to an IP address.
	OnPermissionCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, peer net.IP)
	// OnPermissionDeleted is called after a permission for a given IP address has been
	// removed.
	OnPermissionDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, peer net.IP)
	// OnChannelCreated is called after a new channel has been made. The relay address, the
	// peer address and the channel number can be used to uniquely identify the channel
	// created.
	OnChannelCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr, peer net.Addr, channelNumber uint16)
	// OnChannelDeleted is called after a channel has been removed from the server. The relay
	// address, the peer address and the channel number can be used to uniquely identify the
	// channel deleted.
	OnChannelDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr, peer net.Addr, channelNumber uint16)
}

func genericEventHandler(handlers EventHandlers) allocation.EventHandler { //nolint:cyclop
	return func(arg allocation.EventHandlerArgs) {
		switch arg.Type {
		case allocation.OnAuth:
			if handlers.OnAuth != nil {
				handlers.OnAuth(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.Method, arg.Verdict)
			}
		case allocation.OnAllocationCreated:
			if handlers.OnAllocationCreated != nil {
				handlers.OnAllocationCreated(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.RelayAddr, arg.RequestedPort)
			}
		case allocation.OnAllocationDeleted:
			if handlers.OnAllocationDeleted != nil {
				handlers.OnAllocationDeleted(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm)
			}
		case allocation.OnPermissionCreated:
			if handlers.OnPermissionCreated != nil {
				handlers.OnPermissionCreated(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.RelayAddr, arg.PeerIP)
			}
		case allocation.OnPermissionDeleted:
			if handlers.OnPermissionDeleted != nil {
				handlers.OnPermissionDeleted(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.RelayAddr, arg.PeerIP)
			}
		case allocation.OnChannelCreated:
			if handlers.OnChannelCreated != nil {
				handlers.OnChannelCreated(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.RelayAddr, arg.PeerAddr, arg.ChannelNumber)
			}
		case allocation.OnChannelDeleted:
			if handlers.OnChannelDeleted != nil {
				handlers.OnChannelDeleted(arg.SrcAddr, arg.DstAddr, arg.Protocol.String(),
					arg.Username, arg.Realm, arg.RelayAddr, arg.PeerAddr, arg.ChannelNumber)
			}
		default:
		}
	}
}

// QuotaHandler is a callback allows allocations to be rejected when a per-user quota is
// exceeded. If the callback returns true the allocation request is accepted, otherwise it is
// rejected and a 486 (Allocation Quota Reached) error is returned to the user.
type QuotaHandler func(username, realm string, srcAddr net.Addr) (ok bool)

// ServerConfig configures the Pion TURN Server.
type ServerConfig struct {
	// PacketConnConfigs and ListenerConfigs are a list of all the turn listeners
	// Each listener can have custom behavior around the creation of Relays
	PacketConnConfigs []PacketConnConfig
	ListenerConfigs   []ListenerConfig

	// LoggerFactory must be set for logging from this server.
	LoggerFactory logging.LoggerFactory

	// Realm sets the realm for this server
	Realm string

	// AuthHandler is a callback used to handle incoming auth requests,
	// allowing users to customize Pion TURN with custom behavior
	AuthHandler AuthHandler

	// AuthHandler is a callback used to handle incoming auth requests, allowing users to
	// customize Pion TURN with custom behavior
	QuotaHandler QuotaHandler

	// EventHandlers is a set of callbacks for tracking allocation lifecycle.
	EventHandlers EventHandlers

	// ChannelBindTimeout sets the lifetime of channel binding. Defaults to 10 minutes.
	ChannelBindTimeout time.Duration

	// Sets the server inbound MTU(Maximum transmition unit). Defaults to 1600 bytes.
	InboundMTU int
}

func (s *ServerConfig) validate() error {
	if len(s.PacketConnConfigs) == 0 && len(s.ListenerConfigs) == 0 {
		return errNoAvailableConns
	}

	for _, s := range s.PacketConnConfigs {
		if err := s.validate(); err != nil {
			return err
		}
	}

	for _, s := range s.ListenerConfigs {
		if err := s.validate(); err != nil {
			return err
		}
	}

	return nil
}
