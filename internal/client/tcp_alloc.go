package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/pion/stun"
	"github.com/pion/transport/v2"
	"github.com/pion/turn/v2/internal/proto"
)

var (
	_ transport.TCPListener = (*TCPAllocation)(nil) // Includes type check for net.Listener
	_ transport.Dialer      = (*TCPAllocation)(nil)
)

func noDeadline() time.Time {
	return time.Time{}
}

type TCPAllocation struct {
	connAttemptCh chan *ConnectionAttempt
	acceptTimer   *time.Timer
	Allocation
}

// NewTCPAllocation creates a new instance of TCPConn
func NewTCPAllocation(config *AllocationConfig) *TCPAllocation {
	a := &TCPAllocation{
		connAttemptCh: make(chan *ConnectionAttempt, 10),
		acceptTimer:   time.NewTimer(time.Duration(math.MaxInt64)),
		Allocation: Allocation{
			client:      config.Client,
			relayedAddr: config.RelayedAddr,
			permMap:     newPermissionMap(),
			integrity:   config.Integrity,
			_nonce:      config.Nonce,
			_lifetime:   config.Lifetime,
			log:         config.Log,
		},
	}

	a.log.Debugf("initial lifetime: %d seconds", int(a.lifetime().Seconds()))

	a.refreshAllocTimer = NewPeriodicTimer(
		timerIDRefreshAlloc,
		a.onRefreshTimers,
		a.lifetime()/2,
	)

	a.refreshPermsTimer = NewPeriodicTimer(
		timerIDRefreshPerms,
		a.onRefreshTimers,
		permRefreshInterval,
	)

	if a.refreshAllocTimer.Start() {
		a.log.Debugf("refreshAllocTimer started")
	}
	if a.refreshPermsTimer.Start() {
		a.log.Debugf("refreshPermsTimer started")
	}

	return a
}

func (a *TCPAllocation) Connect(peer net.Addr) (proto.ConnectionID, error) {
	setters := []stun.Setter{
		stun.TransactionID,
		stun.NewType(stun.MethodConnect, stun.ClassRequest),
		addr2PeerAddress(peer),
		a.client.Username(),
		a.client.Realm(),
		a.nonce(),
		a.integrity,
		stun.Fingerprint,
	}

	msg, err := stun.Build(setters...)
	if err != nil {
		return 0, err
	}

	a.log.Debugf("send connect request (peer=%v)", peer)
	trRes, err := a.client.PerformTransaction(msg, a.client.TURNServerAddr(), false)
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

	var cid proto.ConnectionID
	if err := cid.GetFrom(res); err != nil {
		return 0, err
	}

	a.log.Debugf("connect request successful (cid=%v)", cid)
	return cid, nil
}

// Dial connects to the address on the named network.
func (a *TCPAllocation) Dial(network, address string) (net.Conn, error) {
	conn, err := net.Dial(network, a.client.TURNServerAddr().String())
	if err != nil {
		return nil, err
	}

	dataConn, err := a.DialWithConn(conn, network, address)
	if err != nil {
		conn.Close()
	}
	return dataConn, err
}

func (a *TCPAllocation) DialWithConn(conn net.Conn, network, address string) (*TCPConn, error) {
	addr, err := net.ResolveTCPAddr(network, address)
	if err != nil {
		return nil, err
	}

	// Check if we have a permission for the destination IP addr
	perm, ok := a.permMap.find(addr)
	if !ok {
		perm = &permission{}
		a.permMap.insert(addr, perm)
	}

	for i := 0; i < maxRetryAttempts; i++ {
		if err = a.createPermission(perm, addr); !errors.Is(err, errTryAgain) {
			break
		}
	}
	if err != nil {
		return nil, err
	}

	// Send connect request if haven't done so.
	cid, err := a.Connect(addr)
	if err != nil {
		return nil, err
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errTCPAddrCast
	}

	dataConn := &TCPConn{
		TCPConn:       tcpConn,
		remoteAddress: addr,
		allocation:    a,
	}

	if err := a.BindConnection(dataConn, cid); err != nil {
		return nil, fmt.Errorf("failed to bind connection: %w", err)
	}

	return dataConn, nil
}

// BindConnection associates the provided connection
func (a *TCPAllocation) BindConnection(dataConn *TCPConn, cid proto.ConnectionID) error {
	msg, err := stun.Build(
		stun.TransactionID,
		stun.NewType(stun.MethodConnectionBind, stun.ClassRequest),
		cid,
		a.client.Username(),
		a.client.Realm(),
		a.nonce(),
		a.integrity,
		stun.Fingerprint,
	)
	if err != nil {
		return err
	}

	a.log.Debugf("send connectionBind request (cid=%v)", cid)
	_, err = dataConn.Write(msg.Raw)
	if err != nil {
		return err
	}

	// Read exactly one STUN message,
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
		a.log.Debug("connectionBind request successful")
		return nil
	default:
		return fmt.Errorf("unexpected STUN request message: %s", res.String())
	}
}

// Accept waits for and returns the next connection to the listener.
func (a *TCPAllocation) Accept() (net.Conn, error) {
	return a.AcceptTCP()
}

// AcceptTCP accepts the next incoming call and returns the new connection.
func (a *TCPAllocation) AcceptTCP() (transport.TCPConn, error) {
	addr, err := net.ResolveTCPAddr("tcp4", a.client.TURNServerAddr().String())
	if err != nil {
		return nil, err
	}

	tcpConn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	dataConn, err := a.AcceptTCPWithConn(tcpConn)
	if err != nil {
		tcpConn.Close()
	}

	return dataConn, err
}

// AcceptTCP accepts the next incoming call and returns the new connection.
func (a *TCPAllocation) AcceptTCPWithConn(conn net.Conn) (transport.TCPConn, error) {
	select {
	case attempt := <-a.connAttemptCh:

		tcpConn, ok := conn.(*net.TCPConn)
		if !ok {
			return nil, errTCPAddrCast
		}

		dataConn := &TCPConn{
			TCPConn:       tcpConn,
			ConnectionID:  attempt.cid,
			remoteAddress: attempt.from,
			allocation:    a,
		}

		if err := a.BindConnection(dataConn, attempt.cid); err != nil {
			return nil, fmt.Errorf("failed to bind connection: %w", err)
		}

		return dataConn, nil
	case <-a.acceptTimer.C:
		return nil, &net.OpError{
			Op:   "accept",
			Net:  a.Addr().Network(),
			Addr: a.Addr(),
			Err:  newTimeoutError("i/o timeout"),
		}
	}
}

func (a *TCPAllocation) SetDeadline(t time.Time) error {
	var d time.Duration
	if t == noDeadline() {
		d = time.Duration(math.MaxInt64)
	} else {
		d = time.Until(t)
	}
	a.acceptTimer.Reset(d)
	return nil
}

// Close releases the allocation
// Any blocked Accept operations will be unblocked and return errors.
// Any opened connection via Dial/Accept will be closed.
func (a *TCPAllocation) Close() error {
	a.refreshAllocTimer.Stop()
	a.refreshPermsTimer.Stop()

	a.client.OnDeallocated(a.relayedAddr)
	return a.refreshAllocation(0, true /* dontWait=true */)
}

func (a *TCPAllocation) Addr() net.Addr {
	return a.relayedAddr
}

func (a *TCPAllocation) HandleConnectionAttempt(from *net.TCPAddr, cid proto.ConnectionID) error {
	a.connAttemptCh <- &ConnectionAttempt{
		from: from,
		cid:  cid,
	}
	return nil
}
