package turn

import (
	b64 "encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/transport/vnet"
	"github.com/kannonski/turn/v2/internal/client"
	"github.com/kannonski/turn/v2/internal/proto"
)

const (
	defaultRTO        = 200 * time.Millisecond
	maxRtxCount       = 7              // total 7 requests (Rc)
	maxDataBufferSize = math.MaxUint16 //message size limit for Chromium
)

const (
	ProtoUDP = proto.ProtoUDP
	ProtoTCP = proto.ProtoTCP
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
	TURNServerAddr string // TURN server addrees (e.g. "turn.abc.com:3478")
	Username       string
	Password       string
	Realm          string
	Software       string
	RTO            time.Duration
	Conn           net.PacketConn // Listening socket (net.PacketConn)
	LoggerFactory  logging.LoggerFactory
	Net            *vnet.Net

	TransportProtocol        proto.Protocol     // Protocol to peer, UDP: 17 (default) or TCP: 6
	ConnectionAttemptHandler func(ConnectionID) // Incoming TCP connections
}

// Client is a STUN server client
type Client struct {
	conn          net.PacketConn         // read-only
	stunServ      net.Addr               // read-only
	turnServ      net.Addr               // read-only
	stunServStr   string                 // read-only, used for dmuxing
	turnServStr   string                 // read-only, used for dmuxing
	username      stun.Username          // read-only
	password      string                 // read-only
	realm         stun.Realm             // read-only
	integrity     stun.MessageIntegrity  // read-only
	software      stun.Software          // read-only
	trMap         *client.TransactionMap // thread-safe
	rto           time.Duration          // read-only
	relayedConn   *client.UDPConn        // protected by mutex ***
	allocTryLock  client.TryLock         // thread-safe
	listenTryLock client.TryLock         // thread-safe
	net           *vnet.Net              // read-only
	mutex         sync.RWMutex           // thread-safe
	mutexTrMap    sync.Mutex             // thread-safe
	log           logging.LeveledLogger  // read-only

	transportProtocol proto.Protocol
	nonce             stun.Nonce
	caHandler         func(ConnectionID)
}

// NewClient returns a new Client instance. listeningAddress is the address and port to listen on, default "0.0.0.0:0"
func NewClient(config *ClientConfig) (*Client, error) {
	loggerFactory := config.LoggerFactory
	if loggerFactory == nil {
		loggerFactory = logging.NewDefaultLoggerFactory()
	}

	log := loggerFactory.NewLogger("turnc")

	if config.Conn == nil {
		return nil, fmt.Errorf("conn cannot not be nil")
	}

	if config.Net == nil {
		config.Net = vnet.NewNet(nil) // defaults to native operation
	} else if config.Net.IsVirtual() {
		log.Warn("vnet is enabled")
	}

	switch config.TransportProtocol {
	case 0:
		config.TransportProtocol = ProtoUDP
	case proto.ProtoTCP, proto.ProtoUDP:
	default:
		return nil, fmt.Errorf("unsupported protocol: %v", config.TransportProtocol)
	}

	var stunServ, turnServ net.Addr
	var stunServStr, turnServStr string
	var err error
	if len(config.STUNServerAddr) > 0 {
		log.Debugf("resolving %s", config.STUNServerAddr)
		switch config.TransportProtocol {
		case ProtoUDP:
			stunServ, err = config.Net.ResolveUDPAddr("udp4", config.STUNServerAddr)
			if err != nil {
				return nil, err
			}
		case ProtoTCP:
			// TODO: switch to vnet
			stunServ, err = net.ResolveTCPAddr("tcp4", config.STUNServerAddr)
			if err != nil {
				return nil, err
			}
		}
		stunServStr = stunServ.String()
		log.Debugf("stunServ: %s", stunServStr)
	}
	if len(config.TURNServerAddr) > 0 {
		log.Debugf("resolving %s", config.TURNServerAddr)
		switch config.TransportProtocol {
		case ProtoUDP:
			turnServ, err = config.Net.ResolveUDPAddr("udp4", config.TURNServerAddr)
			if err != nil {
				return nil, err
			}
		case ProtoTCP:
			// TODO: switch to vnet
			turnServ, err = net.ResolveTCPAddr("tcp4", config.TURNServerAddr)
			if err != nil {
				return nil, err
			}
		}
		turnServStr = turnServ.String()
		log.Debugf("turnServ: %s", turnServStr)
	}

	rto := defaultRTO
	if config.RTO > 0 {
		rto = config.RTO
	}

	c := &Client{
		conn:              config.Conn,
		stunServ:          stunServ,
		turnServ:          turnServ,
		stunServStr:       stunServStr,
		turnServStr:       turnServStr,
		username:          stun.NewUsername(config.Username),
		password:          config.Password,
		realm:             stun.NewRealm(config.Realm),
		software:          stun.NewSoftware(config.Software),
		net:               config.Net,
		trMap:             client.NewTransactionMap(),
		rto:               rto,
		log:               log,
		transportProtocol: config.TransportProtocol,
		caHandler:         config.ConnectionAttemptHandler,
	}

	return c, nil
}

// TURNServerAddr return the TURN server address
func (c *Client) TURNServerAddr() net.Addr {
	return c.turnServ
}

// STUNServerAddr return the STUN server address
func (c *Client) STUNServerAddr() net.Addr {
	return c.stunServ
}

// Username returns username
func (c *Client) Username() stun.Username {
	return c.username
}

// Realm return realm
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
		return fmt.Errorf("already listening: %s", err.Error())
	}

	go func() {
		buf := make([]byte, maxDataBufferSize)
		for {
			n, from, err := c.conn.ReadFrom(buf)
			if err != nil {
				c.log.Debugf("exiting read loop: %s", err.Error())
				break
			}

			_, err = c.HandleInbound(buf[:n], from)
			if err != nil {
				c.log.Debugf("exiting read loop: %s", err.Error())
				break
			}
		}

		c.listenTryLock.Unlock()
	}()

	return nil
}

// Close closes this client
func (c *Client) Close() {
	c.mutexTrMap.Lock()
	defer c.mutexTrMap.Unlock()

	c.trMap.CloseAndDeleteAll()
}

// TransactionID & Base64: https://play.golang.org/p/EEgmJDI971P

// SendBindingRequestTo sends a new STUN request to the given transport address
func (c *Client) SendBindingRequestTo(to net.Addr) (net.Addr, error) {
	attrs := []stun.Setter{stun.TransactionID, stun.BindingRequest}
	if len(c.software) > 0 {
		attrs = append(attrs, c.software)
	}

	msg, err := stun.Build(attrs...)
	if err != nil {
		return nil, err
	}
	trRes, err := c.PerformTransaction(msg, to, false)
	if err != nil {
		return nil, err
	}

	var reflAddr stun.XORMappedAddress
	if err := reflAddr.GetFrom(trRes.Msg); err != nil {
		return nil, err
	}

	//return fmt.Sprintf("pkt_size=%d src_addr=%s refl_addr=%s:%d", size, addr, reflAddr.IP, reflAddr.Port), nil
	return &net.UDPAddr{
		IP:   reflAddr.IP,
		Port: reflAddr.Port,
	}, nil
}

// SendBindingRequest sends a new STUN request to the STUN server
func (c *Client) SendBindingRequest() (net.Addr, error) {
	if c.stunServ == nil {
		return nil, fmt.Errorf("STUN server address is not set for the client")
	}
	return c.SendBindingRequestTo(c.stunServ)
}

// Allocate sends a TURN allocation request to the given transport address
func (c *Client) Allocate() (net.PacketConn, error) {
	if err := c.allocTryLock.Lock(); err != nil {
		return nil, fmt.Errorf("only one Allocate() caller is allowed: %s", err.Error())
	}
	defer c.allocTryLock.Unlock()

	relayedConn := c.relayedUDPConn()
	if relayedConn != nil {
		return nil, fmt.Errorf("already allocated at %s", relayedConn.LocalAddr().String())
	}

	msg, err := stun.Build(
		stun.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassRequest),
		proto.RequestedTransport{Protocol: c.transportProtocol},
		stun.Fingerprint,
	)
	if err != nil {
		return nil, err
	}

	trRes, err := c.PerformTransaction(msg, c.turnServ, false)
	if err != nil {
		return nil, err
	}

	res := trRes.Msg

	// Anonymous allocate failed, trying to authenticate.
	if err = c.nonce.GetFrom(res); err != nil {
		return nil, err
	}
	if err = c.realm.GetFrom(res); err != nil {
		return nil, err
	}
	c.realm = append([]byte(nil), c.realm...)
	c.integrity = stun.NewLongTermIntegrity(
		c.username.String(), c.realm.String(), c.password,
	)
	// Trying to authorize.
	msg, err = stun.Build(
		stun.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassRequest),
		proto.RequestedTransport{Protocol: c.transportProtocol},
		&c.username,
		&c.realm,
		&c.nonce,
		&c.integrity,
		stun.Fingerprint,
	)
	if err != nil {
		return nil, err
	}

	trRes, err = c.PerformTransaction(msg, c.turnServ, false)
	if err != nil {
		return nil, err
	}
	res = trRes.Msg

	if res.Type.Class == stun.ClassErrorResponse {
		var code stun.ErrorCodeAttribute
		if err = code.GetFrom(res); err == nil {
			return nil, fmt.Errorf("%s (error %s)", res.Type, code)
		}
		return nil, fmt.Errorf("%s", res.Type)
	}

	// Getting relayed addresses from response.
	var relayed proto.RelayedAddress
	if err := relayed.GetFrom(res); err != nil {
		return nil, err
	}
	relayedAddr := &net.UDPAddr{
		IP:   relayed.IP,
		Port: relayed.Port,
	}

	// Getting lifetime from response
	var lifetime proto.Lifetime
	if err := lifetime.GetFrom(res); err != nil {
		return nil, err
	}

	relayedConn = client.NewUDPConn(&client.UDPConnConfig{
		Observer:    c,
		RelayedAddr: relayedAddr,
		Integrity:   c.integrity,
		Nonce:       c.nonce,
		Lifetime:    lifetime.Duration,
		Log:         c.log,
	})

	c.setRelayedUDPConn(relayedConn)

	return relayedConn, nil
}

// PerformTransaction performs STUN transaction
func (c *Client) PerformTransaction(msg *stun.Message, to net.Addr, ignoreResult bool) (client.TransactionResult,
	error) {
	trKey := b64.StdEncoding.EncodeToString(msg.TransactionID[:])

	raw := make([]byte, len(msg.Raw))
	copy(raw, msg.Raw)

	tr := client.NewTransaction(&client.TransactionConfig{
		Key:          trKey,
		Raw:          raw,
		To:           to,
		Interval:     c.rto,
		IgnoreResult: ignoreResult,
	})

	c.trMap.Insert(trKey, tr)

	c.log.Tracef("start %s transaction %s to %s", msg.Type, trKey, tr.To.String())
	_, err := c.conn.WriteTo(tr.Raw, to)
	if err != nil {
		return client.TransactionResult{}, err
	}

	tr.StartRtxTimer(c.onRtxTimeout)

	// If dontWait is true, get the transaction going and return immediately
	if ignoreResult {
		return client.TransactionResult{}, nil
	}

	res := tr.WaitForResult()
	if res.Err != nil {
		return res, res.Err
	}
	return res, nil
}

// OnDeallocated is called when deallocation of relay address has been complete.
// (Called by UDPConn)
func (c *Client) OnDeallocated(relayedAddr net.Addr) {
	c.setRelayedUDPConn(nil)
}

// HandleInbound handles data received.
// This method handles incoming packet demultiplex it by the source address
// and the types of the message.
// This return a booleen (handled or not) and if there was an error.
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
		return true, c.handleChannelData(data)
	case len(c.stunServStr) != 0 && from.String() == c.stunServStr:
		// received from STUN server but it is not a STUN message
		return true, fmt.Errorf("non-STUN message from STUN server")
	default:
		// assume, this is an application data
		c.log.Tracef("non-STUN/TURN packect, unhandled")
	}

	return false, nil
}

func (c *Client) handleSTUNMessage(data []byte, from net.Addr) error {
	raw := make([]byte, len(data))
	copy(raw, data)

	msg := &stun.Message{Raw: raw}
	if err := msg.Decode(); err != nil {
		return fmt.Errorf("failed to decode STUN message: %s", err.Error())
	}

	if msg.Type.Class == stun.ClassRequest {
		return fmt.Errorf("unexpected STUN request message: %s", msg.String())
	}

	if msg.Type.Class == stun.ClassIndication {
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

			c.log.Debugf("data indication received from %s", from.String())

			relayedConn := c.relayedUDPConn()
			if relayedConn == nil {
				c.log.Debug("no relayed conn allocated")
				return nil // silently discard
			}

			relayedConn.HandleInbound(data, from)
		case stun.MethodConnectionAttempt:
			var cid ConnectionID
			if err := cid.GetFrom(msg); err != nil {
				return err
			}

			if c.caHandler != nil {
				c.caHandler(cid)
			}
		}
		return nil
	}

	// This is a STUN response message (transactional)
	// The type is either:
	// - stun.ClassSuccessResponse
	// - stun.ClassErrorResponse

	trKey := b64.StdEncoding.EncodeToString(msg.TransactionID[:])

	c.mutexTrMap.Lock()
	tr, ok := c.trMap.Find(trKey)
	if !ok {
		c.mutexTrMap.Unlock()
		// silently discard
		c.log.Debugf("no transaction for %s", msg.String())
		return nil
	}

	// End the transaction
	tr.StopRtxTimer()
	c.trMap.Delete(trKey)
	c.mutexTrMap.Unlock()

	if !tr.WriteResult(client.TransactionResult{
		Msg:     msg,
		From:    from,
		Retries: tr.Retries(),
	}) {
		c.log.Debugf("no listener for %s", msg.String())
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
		c.log.Debug("no relayed conn allocated")
		return nil // silently discard
	}

	addr, ok := relayedConn.FindAddrByChannelNumber(uint16(chData.Number))
	if !ok {
		return fmt.Errorf("binding with channel %d not found", int(chData.Number))
	}

	c.log.Tracef("channel data received from %s (ch=%d)", addr.String(), int(chData.Number))

	relayedConn.HandleInbound(chData.Data, addr)
	return nil
}

func (c *Client) onRtxTimeout(trKey string, nRtx int) {
	c.mutexTrMap.Lock()
	defer c.mutexTrMap.Unlock()

	tr, ok := c.trMap.Find(trKey)
	if !ok {
		return // already gone
	}

	if nRtx == maxRtxCount {
		// all retransmisstions failed
		c.trMap.Delete(trKey)
		if !tr.WriteResult(client.TransactionResult{
			Err: fmt.Errorf("all retransmissions for %s failed", trKey),
		}) {
			c.log.Debug("no listener for transaction")
		}
		return
	}

	c.log.Tracef("retransmitting transaction %s to %s (nRtx=%d)",
		trKey, tr.To.String(), nRtx)
	_, err := c.conn.WriteTo(tr.Raw, tr.To)
	if err != nil {
		c.trMap.Delete(trKey)
		if !tr.WriteResult(client.TransactionResult{
			Err: fmt.Errorf("failed to retransmit transaction %s", trKey),
		}) {
			c.log.Debug("no listener for transaction")
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

// Connect initiates a new TCP connection to a peer
func (c *Client) Connect(peer *net.TCPAddr) (ConnectionID, error) {
	msg := stun.New()
	msg.WriteHeader()
	stun.TransactionID.AddTo(msg)
	stun.NewType(stun.MethodConnect, stun.ClassRequest).AddTo(msg)
	stun.XORMappedAddress{
		IP:   peer.IP,
		Port: peer.Port,
	}.AddToAs(msg, stun.AttrXORPeerAddress)
	c.username.AddTo(msg)
	c.nonce.AddTo(msg)
	c.realm.AddTo(msg)
	c.integrity.AddTo(msg)
	stun.Fingerprint.AddTo(msg)

	trRes, err := c.PerformTransaction(msg, c.turnServ, false)
	if err != nil {
		return 0, err
	}
	res := trRes.Msg

	if res.Type.Class == stun.ClassErrorResponse {
		var code stun.ErrorCodeAttribute
		if err = code.GetFrom(res); err == nil {
			return 0, fmt.Errorf("%s (error %s)", res.Type, code)
		}
		return 0, fmt.Errorf("%s", res.Type)
	}

	var cid ConnectionID
	err = cid.GetFrom(res)
	if err != nil {
		return 0, err
	}

	return cid, nil
}

// ConnectionBind associates the given tcp connection with the remote connection ID.
// After a successful return the connection can be used normally.
func (c *Client) ConnectionBind(dataConn net.Conn, cid ConnectionID) error {
	msg, err := stun.Build(
		stun.TransactionID,
		stun.NewType(stun.MethodConnectionBind, stun.ClassRequest),
		cid,
		&c.username,
		&c.realm,
		&c.nonce,
		&c.integrity,
		stun.Fingerprint,
	)
	if err != nil {
		return err
	}

	_, err = dataConn.Write(msg.Raw)
	if err != nil {
		return err
	}

	// read exactly one STUN message,
	// any data after belongs to the user
	b := make([]byte, stunHeaderSize)
	n, err := dataConn.Read(b)
	if n != stunHeaderSize {
		return errIncompleteTURNFrame
	} else if err != nil {
		return err
	}
	if !stun.IsMessage(b) {
		return errInvalidTURNFrame
	}

	datagramSize := binary.BigEndian.Uint16(b[2:4]) + stunHeaderSize
	raw := make([]byte, datagramSize)
	copy(raw, b)
	_, err = dataConn.Read(raw[stunHeaderSize:])
	if err != nil {
		return err
	}
	res := &stun.Message{Raw: raw}
	if err := res.Decode(); err != nil {
		return fmt.Errorf("failed to decode STUN message: %s", err.Error())
	}

	switch res.Type.Class {
	case stun.ClassErrorResponse:
		var code stun.ErrorCodeAttribute
		if err = code.GetFrom(res); err == nil {
			return fmt.Errorf("%s (error %s)", res.Type, code)
		}
		return fmt.Errorf("%s", res.Type)
	case stun.ClassSuccessResponse:
		return nil
	default:
		return fmt.Errorf("unexpected STUN request message: %s", res.String())
	}
}

type ConnectionID uint32

func (c ConnectionID) AddTo(m *stun.Message) error {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(c))
	m.Add(stun.AttrConnectionID, b)
	return nil
}

func (c *ConnectionID) GetFrom(m *stun.Message) error {
	b, err := m.Get(stun.AttrConnectionID)
	if err != nil {
		return err
	}
	*c = ConnectionID(binary.BigEndian.Uint32(b))
	return nil
}
