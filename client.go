// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	b64 "encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5"
	"github.com/pion/transport/v5/stdnet"
	"github.com/pion/turn/v5/internal/client"
	"github.com/pion/turn/v5/internal/proto"
)

const (
	defaultRTO        = 200 * time.Millisecond
	maxRtxCount       = 7              // Total 7 requests (Rc)
	maxDataBufferSize = math.MaxUint16 // Message size limit for Chromium

	// maxAlternateRedirects bounds the 300 (Try Alternate) redirects one Allocate
	// follows, matching net/http's redirect limit.
	maxAlternateRedirects = 10
)

//              interval [msec]
// 0: 0 ms      +500
// 1: 500 ms	+1000
// 2: 1500 ms   +2000
// 3: 3500 ms   +4000
// 4: 7500 ms   +8000
// 5: 15500 ms  +16000
// 6: 31500 ms  +32000
// -: 63500 ms  failed

// ClientConfig is a bag of config parameters for Client.
type ClientConfig struct {
	STUNServerAddr string // STUN server address (e.g. "stun.abc.com:3478")
	TURNServerAddr string // TURN server address (e.g. "turn.abc.com:3478")
	Username       string
	Password       string //nolint:gosec // runtime credential, not hardcoded.
	Realm          string
	Software       string
	RTO            time.Duration
	Conn           net.PacketConn // Listening socket (net.PacketConn)
	Net            transport.Net
	LoggerFactory  logging.LoggerFactory

	// PermissionTimeout sets the refresh interval of permissions. Defaults to 2 minutes.
	PermissionRefreshInterval time.Duration

	// RequestedAddressFamily is the address family to request in allocations (IPv4 or IPv6).
	// If not specified (zero value), the client will attempt to infer from the PacketConn's
	// local address, falling back to IPv4 if inference fails. See RFC 6156.
	RequestedAddressFamily RequestedAddressFamily

	// TryAlternateHandler is called before Allocate follows a 300 (Try Alternate)
	// redirect, which it does over plain UDP only. If nil, redirects are followed
	// with the current credentials.
	TryAlternateHandler TryAlternateHandler

	evenPort               bool   // If EVEN-PORT Attribute should be sent in Allocation
	reservationToken       []byte // If Server responds with RESERVATION-TOKEN or if Client wishes to send one
	bindingRefreshInterval time.Duration
	bindingCheckInterval   time.Duration
}

// Client is a STUN server client.
type Client struct {
	conn           net.PacketConn // Read-only
	net            transport.Net  // Read-only
	stunServerAddr net.Addr       // Read-only
	turnServerAddr net.Addr       // Protected by mutex ***

	username      stun.Username          // Protected by mutex ***
	password      string                 // Protected by mutex ***
	realm         stun.Realm             // Read-only
	integrity     stun.MessageIntegrity  // Read-only
	software      stun.Software          // Read-only
	trMap         *client.TransactionMap // Thread-safe
	rto           time.Duration          // Read-only
	relayedConn   *client.UDPConn        // Protected by mutex ***
	tcpAllocation *client.TCPAllocation  // Protected by mutex ***
	allocTryLock  client.TryLock         // Thread-safe
	listenTryLock client.TryLock         // Thread-safe
	mutex         sync.RWMutex           // Thread-safe
	mutexTrMap    sync.Mutex             // Thread-safe
	log           logging.LeveledLogger  // Read-only

	// If EVEN-PORT Attribute should be sent in Allocation
	evenPort bool

	// If Server responds with RESERVATION-TOKEN or if Client wishes to send one
	reservationToken []byte

	// REQUESTED-ADDRESS-FAMILY attribute for allocations (RFC 6156)
	requestedAddressFamily proto.RequestedAddressFamily

	permissionRefreshInterval time.Duration
	bindingRefreshInterval    time.Duration
	bindingCheckInterval      time.Duration

	tryAlternateHandler TryAlternateHandler // Read-only
}

// TryAlternateHandler is called with the context of the Allocate call before it follows
// redirect. It returns the credentials for redirect.Alternate, or an error that Allocate
// returns instead; returning redirect itself declines the redirect.
type TryAlternateHandler func(ctx context.Context, redirect *TryAlternateError) (username, password string, err error)

// TryAlternateError is a 300 (Try Alternate) Allocate error response (RFC 8489 Section 10).
// Allocate returns it, possibly wrapped, for redirects it does not follow, which include
// every redirect over TCP, TLS and DTLS: the caller has to connect to Alternate itself.
// It unwraps to the *stun.TurnError.
type TryAlternateError struct {
	Server    net.Addr // Server that sent the redirect.
	Alternate net.Addr // ALTERNATE-SERVER in Server's address family, nil if none is valid; a *net.TCPAddr over TCP.
	Domain    string   // ALTERNATE-DOMAIN to verify Alternate's certificate with; if empty, the original domain applies.

	turnErr *stun.TurnError
}

func (e *TryAlternateError) Error() string {
	if e.Alternate == nil {
		return fmt.Sprintf("turn: %v answered 300 (Try Alternate) without a valid ALTERNATE-SERVER", e.Server)
	}

	return fmt.Sprintf("turn: %v answered 300 (Try Alternate) with alternate server %v", e.Server, e.Alternate)
}

// Unwrap returns the *stun.TurnError of the response.
func (e *TryAlternateError) Unwrap() error {
	if e.turnErr == nil {
		return nil
	}

	return e.turnErr
}

// allocationTarget is the server and credentials an Allocate request is sent with.
type allocationTarget struct {
	server   net.Addr
	username stun.Username
	password string
}

// inferAddressFamilyFromConn attempts to determine the address
// family (IPv4 or IPv6) from a PacketConn's local address.
// Returns an error if the address type is not IP-based.
func inferAddressFamilyFromConn(
	conn net.PacketConn,
) (proto.RequestedAddressFamily, error) {
	addr := conn.LocalAddr()

	switch a := addr.(type) {
	case *net.UDPAddr:
		if a.IP.To4() != nil {
			return proto.RequestedFamilyIPv4, nil
		}

		return proto.RequestedFamilyIPv6, nil
	case *net.TCPAddr:
		if a.IP.To4() != nil {
			return proto.RequestedFamilyIPv4, nil
		}

		return proto.RequestedFamilyIPv6, nil
	default:
		return 0, fmt.Errorf("cannot infer address family from %T", addr) //nolint:err113
	}
}

// getRequestedAddressFamily determines the address family to use
// for TURN allocations. It follows this priority:
//  1. Use explicitly configured RequestedAddressFamily if set
//  2. Try to infer from the PacketConn's local address
//  3. Fall back to IPv4 default per RFC 6156
func getRequestedAddressFamily(
	log logging.LeveledLogger,
	config *ClientConfig,
) proto.RequestedAddressFamily {
	// If explicitly set, use it
	if config.RequestedAddressFamily != 0 {
		return config.RequestedAddressFamily
	}

	// Try to infer from the PacketConn
	if inferred, err := inferAddressFamilyFromConn(config.Conn); err == nil {
		log.Debugf("Inferred address family %v from connection", inferred)

		return inferred
	}

	log.Debugf("Could not infer address family, defaulting to IPv4")

	// Default to IPv4 per RFC 6156
	return proto.RequestedFamilyIPv4
}

// appendRequestedAddressFamilyOrReservation adds either RESERVATION-TOKEN or
// REQUESTED-ADDRESS-FAMILY to the provided setters slice, respecting mutual
// exclusivity rules from RFC 6156.
// The REQUESTED-ADDRESS-FAMILY attribute is
// only included when IPv6 is desired.
func appendRequestedAddressFamilyOrReservation(
	setters []stun.Setter,
	requestedFamily proto.RequestedAddressFamily,
	reservationToken []byte,
) []stun.Setter {
	// Clients MUST NOT include a REQUESTED-ADDRESS-FAMILY attribute in an
	// Allocate request that contains a RESERVATION-TOKEN attribute.
	// https://www.rfc-editor.org/rfc/rfc6156#section-4.1
	if len(reservationToken) != 0 {
		return append(setters, proto.ReservationToken(reservationToken))
	}

	// Only include the attribute when IPv6 is explicitly requested.
	// This indirectly implied by the specification:
	// If the REQUESTED-ADDRESS-FAMILY attribute is absent, the server MUST
	// allocate an IPv4-relayed transport address for the TURN client.
	// https://www.rfc-editor.org/rfc/rfc6156#section-4.2
	if requestedFamily == proto.RequestedFamilyIPv6 {
		return append(setters, requestedFamily)
	}

	return setters
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on,
// default "0.0.0.0:0".
func NewClient(config *ClientConfig) (*Client, error) { //nolint:gocyclo,cyclop
	loggerFactory := config.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = logging.NewDefaultLoggerFactory()
	}

	log := loggerFactory.NewLogger("turnc")

	if config.Conn == nil {
		return nil, errNilConn
	}

	rto := defaultRTO
	if config.RTO > 0 {
		rto = config.RTO
	}

	if config.Net == nil {
		n, err := stdnet.NewNet()
		if err != nil {
			return nil, err
		}
		config.Net = n
	}

	// Determine the requested address family (RFC 6156)
	requestedAddressFamily := getRequestedAddressFamily(log, config)

	var stunServ, turnServ net.Addr
	var err error

	if len(config.STUNServerAddr) > 0 {
		stunServ, err = config.Net.ResolveUDPAddr("udp", config.STUNServerAddr)
		if err != nil {
			return nil, err
		}

		log.Debugf("Resolved STUN server %s to %s", config.STUNServerAddr, stunServ)
	}

	if len(config.TURNServerAddr) > 0 {
		turnServ, err = config.Net.ResolveUDPAddr("udp", config.TURNServerAddr)
		if err != nil {
			return nil, err
		}

		log.Debugf("Resolved TURN server %s to %s", config.TURNServerAddr, turnServ)
	}

	client := &Client{
		conn:                      config.Conn,
		stunServerAddr:            stunServ,
		turnServerAddr:            turnServ,
		username:                  stun.NewUsername(config.Username),
		password:                  config.Password,
		realm:                     stun.NewRealm(config.Realm),
		software:                  stun.NewSoftware(config.Software),
		trMap:                     client.NewTransactionMap(),
		net:                       config.Net,
		rto:                       rto,
		log:                       log,
		evenPort:                  config.evenPort,
		reservationToken:          config.reservationToken,
		requestedAddressFamily:    requestedAddressFamily,
		permissionRefreshInterval: config.PermissionRefreshInterval,
		bindingRefreshInterval:    config.bindingRefreshInterval,
		bindingCheckInterval:      config.bindingCheckInterval,
		tryAlternateHandler:       config.TryAlternateHandler,
	}

	return client, nil
}

// TURNServerAddr return the TURN server address, which is the alternate server
// once Allocate has followed a 300 (Try Alternate) redirect.
func (c *Client) TURNServerAddr() net.Addr {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.turnServerAddr
}

// STUNServerAddr return the STUN server address.
func (c *Client) STUNServerAddr() net.Addr {
	return c.stunServerAddr
}

// Username returns username.
func (c *Client) Username() stun.Username {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.username
}

// Realm return realm.
func (c *Client) Realm() stun.Realm {
	return c.realm
}

// WriteTo sends data to the specified destination using the base socket.
func (c *Client) WriteTo(data []byte, to net.Addr) (int, error) {
	return c.conn.WriteTo(data, to)
}

// Listen will have this client start listening on the conn provided via the config.
// This is optional. If not used, you will need to call HandleInbound method
// to supply incoming data, instead.
func (c *Client) Listen() error {
	if err := c.listenTryLock.Lock(); err != nil {
		return fmt.Errorf("%w: %s", errAlreadyListening, err.Error())
	}

	go func() {
		buf := make([]byte, maxDataBufferSize)
		for {
			n, from, err := c.conn.ReadFrom(buf)
			if err != nil {
				c.log.Debugf("Failed to read: %s. Exiting loop", err)

				break
			}

			_, err = c.HandleInbound(buf[:n], from)
			if err != nil {
				c.log.Debugf("Failed to handle inbound message: %s. Exiting loop", err)

				break
			}
		}

		c.listenTryLock.Unlock()
	}()

	return nil
}

// Close closes this client.
func (c *Client) Close() {
	c.mutexTrMap.Lock()
	defer c.mutexTrMap.Unlock()

	c.trMap.CloseAndDeleteAll()
}

// TransactionID & Base64: https://play.golang.org/p/EEgmJDI971P

// SendBindingRequestTo sends a new STUN request to the given transport address.
func (c *Client) SendBindingRequestTo(to net.Addr) (net.Addr, error) {
	attrs := []stun.Setter{stun.TransactionID, stun.BindingRequest}
	if len(c.software) > 0 {
		attrs = append(attrs, c.software)
	}

	msg, err := stun.Build(attrs...)
	if err != nil {
		return nil, err
	}
	trRes, err := c.PerformTransactionWithContext(msg, to, false, context.TODO())
	if err != nil {
		return nil, err
	}

	var reflAddr stun.XORMappedAddress
	if err := reflAddr.GetFrom(trRes.Msg); err != nil {
		return nil, err
	}

	return &net.UDPAddr{
		IP:   reflAddr.IP,
		Port: reflAddr.Port,
	}, nil
}

// SendBindingRequest sends a new STUN request to the STUN server.
func (c *Client) SendBindingRequest() (net.Addr, error) {
	if c.stunServerAddr == nil {
		return nil, errSTUNServerAddressNotSet
	}

	return c.SendBindingRequestTo(c.stunServerAddr)
}

func (c *Client) sendAllocateRequest(ctx context.Context, protocol proto.Protocol, target allocationTarget) ( //nolint:cyclop
	relayed proto.RelayedAddress,
	lifetime proto.Lifetime,
	nonce stun.Nonce,
	reservationToken proto.ReservationToken,
	err error,
) {
	allocationSetters := []stun.Setter{
		stun.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassRequest),
		proto.RequestedTransport{Protocol: protocol},
	}

	allocationSetters = appendRequestedAddressFamilyOrReservation(
		allocationSetters, c.requestedAddressFamily, c.reservationToken,
	)
	if c.evenPort {
		allocationSetters = append(allocationSetters, proto.EvenPort{ReservePort: true})
	}

	// FINGERPRINT must be the last attribute per RFC 5389
	allocationSetters = append(allocationSetters, stun.Fingerprint)

	msg, err := stun.Build(allocationSetters...)
	if err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}

	trRes, err := c.PerformTransactionWithContext(msg, target.server, false, ctx)
	if err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}

	res := trRes.Msg

	// Only a 401 or 438 challenge is answered with credentials. Other errors end the
	// attempt, including a 300 (Try Alternate) that carries NONCE and REALM.
	var code stun.ErrorCodeAttribute
	if res.Type.Class == stun.ClassErrorResponse && code.GetFrom(res) == nil &&
		code.Code != stun.CodeUnauthorized && code.Code != stun.CodeStaleNonce {
		return relayed, lifetime, nonce, reservationToken, c.allocateError(res, code, target.server)
	}

	// Anonymous allocate failed, trying to authenticate.
	if err = nonce.GetFrom(res); err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}
	if err = c.realm.GetFrom(res); err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}
	c.realm = append([]byte(nil), c.realm...)
	c.integrity = stun.NewLongTermIntegrity(
		target.username.String(), c.realm.String(), target.password,
	)
	// Trying to authorize.
	allocationSetters = []stun.Setter{
		stun.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassRequest),
		proto.RequestedTransport{Protocol: protocol},
		&target.username,
		&c.realm,
		&nonce,
	}

	allocationSetters = appendRequestedAddressFamilyOrReservation(
		allocationSetters, c.requestedAddressFamily, c.reservationToken,
	)
	if c.evenPort {
		allocationSetters = append(allocationSetters, proto.EvenPort{ReservePort: true})
	}

	// MESSAGE-INTEGRITY must follow every attribute it protects. Receivers
	// ignore ordinary attributes that appear after it.
	// https://www.rfc-editor.org/rfc/rfc8489.html#section-9
	allocationSetters = append(allocationSetters, &c.integrity)

	// FINGERPRINT must be the last attribute per RFC 5389
	allocationSetters = append(allocationSetters, stun.Fingerprint)

	msg, err = stun.Build(allocationSetters...)
	if err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}

	trRes, err = c.PerformTransactionWithContext(msg, target.server, false, ctx)
	if err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}
	res = trRes.Msg

	if res.Type.Class == stun.ClassErrorResponse {
		if err = code.GetFrom(res); err == nil {
			return relayed, lifetime, nonce, reservationToken, c.allocateError(res, code, target.server)
		}

		return relayed, lifetime, nonce, reservationToken, fmt.Errorf("%s", res.Type) //nolint:err113
	}

	// Getting relayed addresses from response.
	if err := relayed.GetFrom(res); err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}

	// Getting lifetime from response
	if err := lifetime.GetFrom(res); err != nil {
		return relayed, lifetime, nonce, reservationToken, err
	}

	// Getting reservation-token from response
	if c.evenPort {
		if err := reservationToken.GetFrom(res); err != nil {
			return relayed, lifetime, nonce, reservationToken, err
		}
	}

	return relayed, lifetime, nonce, reservationToken, nil
}

// allocateError returns the error for an Allocate error response; a 300 (Try Alternate)
// becomes a *TryAlternateError.
func (c *Client) allocateError(res *stun.Message, code stun.ErrorCodeAttribute, server net.Addr) error {
	turnErr := &stun.TurnError{StunMessageType: res.Type, ErrorCodeAttr: code}
	if code.Code != stun.CodeTryAlternate {
		return turnErr
	}

	if peer := c.connectedServer(); peer != nil {
		server = peer
	}
	redirect := &TryAlternateError{Server: server, Alternate: c.alternateServer(res, server), turnErr: turnErr}
	if domain, err := res.Get(stun.AttrAlternateDomain); err == nil {
		redirect.Domain = string(domain)
	}

	return redirect
}

// alternateServer returns the first valid ALTERNATE-SERVER in res that shares server's
// address family (RFC 8489 Section 10), typed for the client's transport.
func (c *Client) alternateServer(res *stun.Message, server net.Addr) net.Addr {
	serverIP := addrIP(server)
	for _, attr := range res.Attributes {
		if attr.Type != stun.AttrAlternateServer {
			continue
		}
		alt, ok := parseAlternateServer(attr)
		if !ok || (serverIP != nil && (alt.IP.To4() == nil) != (serverIP.To4() == nil)) {
			continue
		}
		if _, isTCP := c.conn.LocalAddr().(*net.TCPAddr); isTCP {
			return &net.TCPAddr{IP: alt.IP, Port: alt.Port}
		}

		return &net.UDPAddr{IP: alt.IP, Port: alt.Port}
	}

	return nil
}

// parseAlternateServer decodes attr, rejecting truncated, padded and unspecified addresses.
func parseAlternateServer(attr stun.RawAttribute) (stun.AlternateServer, bool) {
	var alt stun.AlternateServer
	err := alt.GetFrom(&stun.Message{Attributes: stun.Attributes{attr}})
	// The value is a 2-byte family and a 2-byte port followed by the address.
	ok := err == nil && len(attr.Value) == 4+len(alt.IP) && !alt.IP.IsUnspecified() && alt.Port != 0

	return alt, ok
}

// addrIP returns the IP of a UDP or TCP address, or nil.
func addrIP(addr net.Addr) net.IP {
	switch addr := addr.(type) {
	case *net.UDPAddr:
		return addr.IP
	case *net.TCPAddr:
		return addr.IP
	default:
		return nil
	}
}

// allocate sends an Allocate request, following 300 (Try Alternate) redirects over packet
// transports. On success the client keeps the server and credentials that granted it.
func (c *Client) allocate(ctx context.Context, protocol proto.Protocol) (
	relayed proto.RelayedAddress,
	lifetime proto.Lifetime,
	nonce stun.Nonce,
	reservationToken proto.ReservationToken,
	err error,
) {
	target := c.allocationTarget()
	tried := map[string]struct{}{}
	for {
		relayed, lifetime, nonce, reservationToken, err = c.sendAllocateRequest(ctx, protocol, target)
		if err == nil {
			c.setAllocationTarget(target)

			return relayed, lifetime, nonce, reservationToken, nil
		}
		if target, err = c.followAlternate(ctx, err, target, tried); err != nil {
			return relayed, lifetime, nonce, reservationToken, err
		}
	}
}

// followAlternate returns where to retry the Allocate request that failed with err,
// or the error Allocate returns when err is not a redirect it follows. tried holds
// the servers this Allocate has sent the request to.
func (c *Client) followAlternate(
	ctx context.Context, err error, target allocationTarget, tried map[string]struct{},
) (allocationTarget, error) {
	var redirect *TryAlternateError
	if !errors.As(err, &redirect) || redirect.Alternate == nil || target.server == nil || c.connectedServer() != nil {
		return target, err
	}

	tried[target.server.String()] = struct{}{}
	if _, ok := tried[redirect.Alternate.String()]; ok {
		return target, fmt.Errorf("%w: %w", errAlternateServerLoop, err)
	}
	if len(tried) > maxAlternateRedirects {
		return target, fmt.Errorf("%w: %w", errTooManyAlternateRedirects, err)
	}

	next := allocationTarget{server: redirect.Alternate, username: target.username, password: target.password}
	if c.tryAlternateHandler != nil {
		username, password, handlerErr := c.tryAlternateHandler(ctx, redirect)
		if handlerErr != nil {
			return target, handlerErr
		}
		next.username, next.password = stun.NewUsername(username), password
	}
	c.log.Debugf("Following 300 (Try Alternate) from %s to %s", target.server, redirect.Alternate)

	return next, nil
}

// AllocateWithContext sends a TURN allocation request to the given transport address.
// The context controls the lifetime of the allocation request and its response.
func (c *Client) AllocateWithContext(ctx context.Context) (net.PacketConn, error) {
	if err := c.allocTryLock.Lock(); err != nil {
		return nil, fmt.Errorf("%w: %s", errOneAllocateOnly, err.Error())
	}
	defer c.allocTryLock.Unlock()

	relayedConn := c.relayedUDPConn()
	if relayedConn != nil {
		return nil, fmt.Errorf("%w: %s", errAlreadyAllocated, relayedConn.LocalAddr().String())
	}

	relayed, lifetime, nonce, reservationToken, err := c.allocate(ctx, proto.ProtoUDP)
	if err != nil {
		return nil, err
	}

	relayedAddr := &net.UDPAddr{
		IP:   relayed.IP,
		Port: relayed.Port,
	}

	//nolint: contextcheck
	relayedConn = client.NewUDPConn(&client.AllocationConfig{
		Client:                    c,
		RelayedAddr:               relayedAddr,
		ServerAddr:                c.TURNServerAddr(),
		Realm:                     c.realm,
		Username:                  c.Username(),
		Integrity:                 c.integrity,
		Nonce:                     nonce,
		Lifetime:                  lifetime.Duration,
		Net:                       c.net,
		Log:                       c.log,
		PermissionRefreshInterval: c.permissionRefreshInterval,
		BindingRefreshInterval:    c.bindingRefreshInterval,
		BindingCheckInterval:      c.bindingCheckInterval,
	})
	c.setRelayedUDPConn(relayedConn)
	c.setReservationToken(reservationToken)

	return relayedConn, nil
}

// Allocate sends a TURN allocation request to the given transport address.
func (c *Client) Allocate() (net.PacketConn, error) {
	return c.AllocateWithContext(context.TODO())
}

// AllocateTCPWithContext creates a new TCP allocation at the TURN server.
// The context controls the lifetime of the allocation request and its response.
func (c *Client) AllocateTCPWithContext(ctx context.Context) (*client.TCPAllocation, error) {
	if err := c.allocTryLock.Lock(); err != nil {
		return nil, fmt.Errorf("%w: %s", errOneAllocateOnly, err.Error())
	}
	defer c.allocTryLock.Unlock()

	allocation := c.getTCPAllocation()
	if allocation != nil {
		return nil, fmt.Errorf("%w: %s", errAlreadyAllocated, allocation.Addr())
	}

	relayed, lifetime, nonce, reservationToken, err := c.allocate(ctx, proto.ProtoTCP)
	if err != nil {
		return nil, err
	}

	relayedAddr := &net.TCPAddr{
		IP:   relayed.IP,
		Port: relayed.Port,
	}

	allocation = client.NewTCPAllocation(&client.AllocationConfig{
		Client:      c,
		RelayedAddr: relayedAddr,
		ServerAddr:  c.TURNServerAddr(),
		Realm:       c.realm,
		Username:    c.Username(),
		Integrity:   c.integrity,
		Nonce:       nonce,
		Lifetime:    lifetime.Duration,
		Net:         c.net,
		Log:         c.log,
	})

	c.setTCPAllocation(allocation)
	c.setReservationToken(reservationToken)

	return allocation, nil
}

// AllocateTCP creates a new TCP allocation at the TURN server.
func (c *Client) AllocateTCP() (*client.TCPAllocation, error) {
	return c.AllocateTCPWithContext(context.TODO())
}

// CreatePermission Issues a CreatePermission request for the supplied addresses
// as described in https://datatracker.ietf.org/doc/html/rfc5766#section-9
func (c *Client) CreatePermission(addrs ...net.Addr) error {
	if conn := c.relayedUDPConn(); conn != nil {
		if err := conn.CreatePermissions(addrs...); err != nil {
			return err
		}
	}

	if allocation := c.getTCPAllocation(); allocation != nil {
		if err := allocation.CreatePermissions(addrs...); err != nil {
			return err
		}
	}

	return nil
}

// PerformTransaction performs STUN transaction.
func (c *Client) PerformTransaction(msg *stun.Message, to net.Addr, ignoreResult bool) (client.TransactionResult, error) {
	return c.PerformTransactionWithContext(msg, to, ignoreResult, context.TODO())
}

// PerformTransactionWithContext performs a STUN transaction with a context.
func (c *Client) PerformTransactionWithContext(msg *stun.Message, to net.Addr, ignoreResult bool, ctx context.Context) (client.TransactionResult,
	error,
) {
	if err := ctx.Err(); err != nil {
		return client.TransactionResult{}, err
	}

	trKey := b64.StdEncoding.EncodeToString(msg.TransactionID[:])

	raw := make([]byte, len(msg.Raw))
	copy(raw, msg.Raw)

	var integrity stun.MessageIntegrity
	if msg.Contains(stun.AttrMessageIntegrity) {
		integrity = append(stun.MessageIntegrity{}, c.integrity...)
	}
	tr := client.NewTransaction(&client.TransactionConfig{
		Integrity:    integrity,
		Key:          trKey,
		Raw:          raw,
		To:           to,
		Interval:     c.rto,
		IgnoreResult: ignoreResult,
	})

	c.trMap.Insert(trKey, tr)

	c.log.Tracef("Start %s transaction %s to %s", msg.Type, trKey, tr.To)
	_, err := c.conn.WriteTo(tr.Raw, to)
	if err != nil {
		c.removeTransaction(trKey)

		return client.TransactionResult{}, err
	}

	tr.StartRtxTimer(c.onRtxTimeout)

	// If ignoreResult is true, get the transaction going and return immediately
	if ignoreResult {
		return client.TransactionResult{}, nil
	}

	defer c.removeTransaction(trKey)

	res := tr.WaitForResult(ctx)
	if res.Err != nil {
		return res, res.Err
	}

	return res, nil
}

func (c *Client) removeTransaction(trKey string) (*client.Transaction, bool) {
	c.mutexTrMap.Lock()
	defer c.mutexTrMap.Unlock()

	tr, ok := c.trMap.Find(trKey)
	if !ok {
		return nil, false
	}

	tr.StopRtxTimer()
	c.trMap.Delete(trKey)

	return tr, true
}

// OnDeallocated is called when de-allocation of relay address has been complete.
// (Called by UDPConn).
func (c *Client) OnDeallocated(net.Addr) {
	c.setRelayedUDPConn(nil)
	c.setTCPAllocation(nil)
}

// HandleInbound handles data received.
// This method handles incoming packet de-multiplex it by the source address
// and the types of the message.
// This return a boolean (handled or not) and if there was an error.
// Caller should check if the packet was handled by this client or not.
// If not handled, it is assumed that the packet is application data.
// If an error is returned, the caller should discard the packet regardless.
func (c *Client) HandleInbound(data []byte, from net.Addr) (bool, error) {
	// +-------------------+-------------------------------+
	// |   Return Values   |                               |
	// +-------------------+       Meaning / Action        |
	// | handled |  error  |                               |
	// |=========+=========+===============================+
	// |  false  |   nil   | Handle the packet as app data |
	// |---------+---------+-------------------------------+
	// |  true   |   nil   |        Nothing to do          |
	// |---------+---------+-------------------------------+
	// |  false  |  error  |     (shouldn't happen)        |
	// |---------+---------+-------------------------------+
	// |  true   |  error  | Error occurred while handling |
	// +---------+---------+-------------------------------+
	// Possible causes of the error:
	//  - Malformed packet (parse error)
	//  - STUN message was a request
	//  - Non-STUN message from the STUN server

	switch {
	case stun.IsMessage(data):
		return true, c.handleSTUNMessage(data, from)
	case proto.IsChannelData(data):
		if !c.isTURNServer(from) {
			return true, nil
		}

		return true, c.handleChannelData(data)
	case c.stunServerAddr != nil && from.String() == c.stunServerAddr.String():
		// Received from STUN server but it is not a STUN message
		return true, errNonSTUNMessage
	default:
		// Assume, this is an application data
		c.log.Tracef("Ignoring non-STUN/TURN packet")
	}

	return false, nil
}

func (c *Client) isTURNServer(from net.Addr) bool {
	server := c.connectedServer()
	if server == nil {
		server = c.TURNServerAddr()
	}

	return from != nil && server != nil && from.String() == server.String()
}

// connectedServer returns the peer of a connection-oriented transport, such as
// TCP, TLS or DTLS, and nil for a packet transport.
func (c *Client) connectedServer() net.Addr {
	if conn, ok := c.conn.(interface{ RemoteAddr() net.Addr }); ok {
		return conn.RemoteAddr()
	}

	return nil
}

func (c *Client) handleSTUNMessage(data []byte, from net.Addr) error { //nolint:cyclop
	raw := make([]byte, len(data))
	copy(raw, data)

	msg := &stun.Message{Raw: raw}
	if err := msg.Decode(); err != nil {
		return fmt.Errorf("%w: %s", errFailedToDecodeSTUN, err.Error())
	}

	if msg.Type.Class == stun.ClassRequest {
		return fmt.Errorf("%w : %s", errUnexpectedSTUNRequestMessage, msg.String())
	}

	if msg.Type.Class == stun.ClassIndication { // nolint:nestif
		if !c.isTURNServer(from) {
			return nil
		}
		switch msg.Type.Method {
		case stun.MethodData:
			var peerAddr proto.PeerAddress
			if err := peerAddr.GetFrom(msg); err != nil {
				return err
			}
			from = &net.UDPAddr{
				IP:   peerAddr.IP,
				Port: peerAddr.Port,
			}

			var data proto.Data
			if err := data.GetFrom(msg); err != nil {
				return err
			}

			c.log.Tracef("Data indication received from %s", from)

			relayedConn := c.relayedUDPConn()
			if relayedConn == nil {
				c.log.Debug("No relayed conn allocated")

				return nil // Silently discard
			}
			relayedConn.HandleInbound(data, from)
		case stun.MethodConnectionAttempt:
			var peerAddr proto.PeerAddress
			if err := peerAddr.GetFrom(msg); err != nil {
				return err
			}

			addr := &net.TCPAddr{
				IP:   peerAddr.IP,
				Port: peerAddr.Port,
			}

			var cid proto.ConnectionID
			if err := cid.GetFrom(msg); err != nil {
				return err
			}

			c.log.Debugf("Connection attempt from %s", addr)

			allocation := c.getTCPAllocation()
			if allocation == nil {
				c.log.Debug("No TCP allocation exists")

				return nil // Silently discard
			}

			allocation.HandleConnectionAttempt(addr, cid)
		default:
			c.log.Debug("Received unsupported STUN method")
		}

		return nil
	}

	// This is a STUN response message (transactional)
	// The type is either:
	// - stun.ClassSuccessResponse
	// - stun.ClassErrorResponse

	trKey := b64.StdEncoding.EncodeToString(msg.TransactionID[:])

	tr, ok := c.trMap.Find(trKey)
	if !ok {
		// Silently discard
		c.log.Debugf("No transaction for %s", msg)

		return nil
	}

	if !tr.CheckIntegrity(msg) {
		return nil
	}
	tr, ok = c.removeTransaction(trKey)
	if !ok {
		return nil
	}

	if !tr.WriteResult(client.TransactionResult{
		Msg:     msg,
		From:    from,
		Retries: tr.Retries(),
	}) {
		c.log.Debugf("No listener for %s", msg)
	}

	return nil
}

func (c *Client) handleChannelData(data []byte) error {
	chData := &proto.ChannelData{
		Raw: make([]byte, len(data)),
	}
	copy(chData.Raw, data)
	if err := chData.Decode(); err != nil {
		return err
	}

	relayedConn := c.relayedUDPConn()
	if relayedConn == nil {
		c.log.Debug("No relayed conn allocated")

		return nil // Silently discard
	}

	addr, ok := relayedConn.FindAddrByChannelNumber(uint16(chData.Number))
	if !ok {
		return fmt.Errorf("%w: %d", errChannelBindNotFound, int(chData.Number))
	}

	c.log.Tracef("Channel data received from %s (ch=%d)", addr.String(), int(chData.Number))

	relayedConn.HandleInbound(chData.Data, addr)

	return nil
}

func (c *Client) onRtxTimeout(trKey string, nRtx int) {
	c.mutexTrMap.Lock()
	defer c.mutexTrMap.Unlock()

	tr, ok := c.trMap.Find(trKey)
	if !ok {
		return // Already gone
	}

	if nRtx == maxRtxCount {
		// All retransmissions failed
		c.trMap.Delete(trKey)
		if !tr.WriteResult(client.TransactionResult{
			Err: fmt.Errorf("%w %s", errAllRetransmissionsFailed, trKey),
		}) {
			c.log.Debug("No listener for transaction")
		}

		return
	}

	c.log.Tracef("Retransmitting transaction %s to %s (nRtx=%d)",
		trKey, tr.To, nRtx)
	_, err := c.conn.WriteTo(tr.Raw, tr.To)
	if err != nil {
		c.trMap.Delete(trKey)
		if !tr.WriteResult(client.TransactionResult{
			Err: fmt.Errorf("%w %s", errFailedToRetransmitTransaction, trKey),
		}) {
			c.log.Debug("No listener for transaction")
		}

		return
	}
	tr.StartRtxTimer(c.onRtxTimeout)
}

func (c *Client) setRelayedUDPConn(conn *client.UDPConn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.relayedConn = conn
}

func (c *Client) relayedUDPConn() *client.UDPConn {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.relayedConn
}

func (c *Client) setTCPAllocation(alloc *client.TCPAllocation) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.tcpAllocation = alloc
}

func (c *Client) getTCPAllocation() *client.TCPAllocation {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.tcpAllocation
}

func (c *Client) setReservationToken(reservationToken []byte) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.reservationToken = reservationToken
}

func (c *Client) getReservationToken() []byte {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.reservationToken
}

func (c *Client) allocationTarget() allocationTarget {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return allocationTarget{server: c.turnServerAddr, username: c.username, password: c.password}
}

func (c *Client) setAllocationTarget(target allocationTarget) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.turnServerAddr, c.username, c.password = target.server, target.username, target.password
}
