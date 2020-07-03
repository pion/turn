package server

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/allocation"
	"github.com/pion/turn/v2/internal/ipnet"
	"github.com/pion/turn/v2/internal/proto"
)

// // https://tools.ietf.org/html/rfc5766#section-6.2
func handleAllocateRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received AllocateRequest from %s", r.SrcAddr.String())

	// 1. The server MUST require that the request be authenticated.  This
	//    authentication MUST be done using the long-term credential
	//    mechanism of [https://tools.ietf.org/html/rfc5389#section-10.2.2]
	//    unless the client and server agree to use another mechanism through
	//    some procedure outside the scope of this document.
	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodAllocate)
	if !hasAuth {
		return err
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	requestedPort := 0
	reservationToken := ""

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})
	insufficentCapacityMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeInsufficientCapacity})

	// 2. The server checks if the 5-tuple is currently in use by an
	//    existing allocation.  If yes, the server rejects the request with
	//    a 437 (Allocation Mismatch) error.
	if alloc := r.AllocationManager.GetAllocation(fiveTuple); alloc != nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeAllocMismatch})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("relay already allocated for 5-TUPLE"), msg...)
	}

	// 3. The server checks if the request contains a REQUESTED-TRANSPORT
	//    attribute.  If the REQUESTED-TRANSPORT attribute is not included
	//    or is malformed, the server rejects the request with a 400 (Bad
	//    Request) error.  Otherwise, if the attribute is included but
	//    specifies a protocol other that UDP, the server rejects the
	//    request with a 442 (Unsupported Transport Protocol) error.
	//
	// Updated by https://tools.ietf.org/html/rfc6062#section-5.1
	// 1. If the REQUESTED-TRANSPORT attribute is included and specifies a
	//    protocol other than UDP or TCP, the server MUST reject the
	//    request with a 442 (Unsupported Transport Protocol) error.  If
	//    the value is UDP, and if UDP transport is allowed by local
	//    policy, the server MUST continue with the procedures of [RFC5766]
	//    instead of this document.  If the value is UDP, and if UDP
	//    transport is forbidden by local policy, the server MUST reject
	//    the request with a 403 (Forbidden) error.
	var requestedTransport proto.RequestedTransport
	if err = requestedTransport.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	} else if requestedTransport.Protocol != proto.ProtoUDP && requestedTransport.Protocol != proto.ProtoTCP {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeUnsupportedTransProto})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("RequestedTransport must be UDP or TCP"), msg...)
	}

	// 4. The request may contain a DONT-FRAGMENT attribute.  If it does,
	//    but the server does not support sending UDP datagrams with the DF
	//    bit set to 1 (see Section 12), then the server treats the DONT-
	//    FRAGMENT attribute in the Allocate request as an unknown
	//    comprehension-required attribute.
	//
	// Updated by https://tools.ietf.org/html/rfc6062#section-5.1
	// 3. If the request contains the DONT-FRAGMENT, EVEN-PORT, or
	//    RESERVATION-TOKEN attribute, the server MUST reject the request
	//    with a 400 (Bad Request) error.
	if m.Contains(stun.AttrDontFragment) {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeUnknownAttribute}, &stun.UnknownAttributes{stun.AttrDontFragment})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("no support for DONT-FRAGMENT"), msg...)
	}

	// 5.  The server checks if the request contains a RESERVATION-TOKEN
	//     attribute.  If yes, and the request also contains an EVEN-PORT
	//     attribute, then the server rejects the request with a 400 (Bad
	//     Request) error.  Otherwise, it checks to see if the token is
	//     valid (i.e., the token is in range and has not expired and the
	//     corresponding relayed transport address is still available).  If
	//     the token is not valid for some reason, the server rejects the
	//     request with a 508 (Insufficient Capacity) error.
	var reservationTokenAttr proto.ReservationToken
	if err = reservationTokenAttr.GetFrom(m); err == nil {
		var evenPort proto.EvenPort
		if err = evenPort.GetFrom(m); err == nil {
			return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("Request must not contain RESERVATION-TOKEN and EVEN-PORT"), badRequestMsg...)
		}

		// Updated by https://tools.ietf.org/html/rfc6062#section-5.1
		// 3. If the request contains the DONT-FRAGMENT, EVEN-PORT, or
		//    RESERVATION-TOKEN attribute, the server MUST reject the request
		//    with a 400 (Bad Request) error.
		if requestedTransport.Protocol == proto.ProtoTCP {
			return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("Request must not contain RESERVATION-TOKEN for TCP"), badRequestMsg...)
		}
	}

	// 6. The server checks if the request contains an EVEN-PORT attribute.
	//    If yes, then the server checks that it can satisfy the request
	//    (i.e., can allocate a relayed transport address as described
	//    below).  If the server cannot satisfy the request, then the
	//    server rejects the request with a 508 (Insufficient Capacity)
	//    error.
	var evenPort proto.EvenPort
	if err = evenPort.GetFrom(m); err == nil {
		// Updated by https://tools.ietf.org/html/rfc6062#section-5.1
		// 3. If the request contains the DONT-FRAGMENT, EVEN-PORT, or
		//    RESERVATION-TOKEN attribute, the server MUST reject the request
		//    with a 400 (Bad Request) error.
		if requestedTransport.Protocol == proto.ProtoTCP {
			return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("Request must not contain EVEN-PORT for TCP"), badRequestMsg...)
		}

		randomPort := 0
		randomPort, err = r.AllocationManager.GetRandomEvenPort()
		if err != nil {
			return buildAndSendErr(r.Conn, r.SrcAddr, err, insufficentCapacityMsg...)
		}
		requestedPort = randomPort
		reservationToken = randSeq(8)
	}

	// 7. At any point, the server MAY choose to reject the request with a
	//    486 (Allocation Quota Reached) error if it feels the client is
	//    trying to exceed some locally defined allocation quota.  The
	//    server is free to define this allocation quota any way it wishes,
	//    but SHOULD define it based on the username used to authenticate
	//    the request, and not on the client's transport address.

	// 8. Also at any point, the server MAY choose to reject the request
	//    with a 300 (Try Alternate) error if it wishes to redirect the
	//    client to a different server.  The use of this error code and
	//    attribute follow the specification in [RFC5389].
	lifetimeDuration := allocationLifeTime(m)
	a, err := r.AllocationManager.CreateAllocation(
		fiveTuple,
		r.Conn,
		requestedPort,
		lifetimeDuration)
	if err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, insufficentCapacityMsg...)
	}

	// Once the allocation is created, the server replies with a success
	// response.  The success response contains:
	//   * An XOR-RELAYED-ADDRESS attribute containing the relayed transport
	//     address.
	//   * A LIFETIME attribute containing the current value of the time-to-
	//     expiry timer.
	//   * A RESERVATION-TOKEN attribute (if a second relayed transport
	//     address was reserved).
	//   * An XOR-MAPPED-ADDRESS attribute containing the client's IP address
	//     and port (from the 5-tuple).

	srcIP, srcPort, err := ipnet.AddrIPPort(r.SrcAddr)
	if err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	relayIP, relayPort, err := ipnet.AddrIPPort(a.RelayAddr)
	if err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	responseAttrs := []stun.Setter{
		&proto.RelayedAddress{
			IP:   relayIP,
			Port: relayPort,
		},
		&proto.Lifetime{
			Duration: lifetimeDuration,
		},
		&stun.XORMappedAddress{
			IP:   srcIP,
			Port: srcPort,
		},
	}

	// Updated by https://tools.ietf.org/html/rfc6062#section-5.1
	// 5.  The RESERVATION-TOKEN attribute MUST NOT be present in the
	//     success response.
	if requestedTransport.Protocol == proto.ProtoUDP && reservationToken != "" {
		r.AllocationManager.CreateReservation(reservationToken, relayPort)
		responseAttrs = append(responseAttrs, proto.ReservationToken([]byte(reservationToken)))
	}

	msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse), append(responseAttrs, messageIntegrity)...)
	return buildAndSend(r.Conn, r.SrcAddr, msg...)
}

func handleRefreshRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received RefreshRequest from %s", r.SrcAddr.String())

	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodRefresh)
	if !hasAuth {
		return err
	}

	lifetimeDuration := allocationLifeTime(m)
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}

	if lifetimeDuration != 0 {
		a := r.AllocationManager.GetAllocation(fiveTuple)

		if a == nil {
			return fmt.Errorf("no allocation found for %v:%v", r.SrcAddr, r.Conn.LocalAddr())
		}
		a.Refresh(lifetimeDuration)
	} else {
		r.AllocationManager.DeleteAllocation(fiveTuple)
	}

	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodRefresh, stun.ClassSuccessResponse), []stun.Setter{
		&proto.Lifetime{
			Duration: lifetimeDuration,
		},
		messageIntegrity,
	}...)...)
}

func handleCreatePermissionRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received CreatePermission from %s", r.SrcAddr.String())

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return fmt.Errorf("no allocation found for %v:%v", r.SrcAddr, r.Conn.LocalAddr())
	}

	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodCreatePermission)
	if !hasAuth {
		return err
	}

	addCount := 0

	if err := m.ForEach(stun.AttrXORPeerAddress, func(m *stun.Message) error {
		var peerAddress proto.PeerAddress
		if err := peerAddress.GetFrom(m); err != nil {
			return err
		}

		r.Log.Debugf("adding permission for %s", fmt.Sprintf("%s:%d",
			peerAddress.IP.String(), peerAddress.Port))
		a.AddPermission(allocation.NewPermission(
			&net.UDPAddr{
				IP:   peerAddress.IP,
				Port: peerAddress.Port,
			},
			r.Log,
		))
		addCount++
		return nil
	}); err != nil {
		addCount = 0
	}

	respClass := stun.ClassSuccessResponse
	if addCount == 0 {
		respClass = stun.ClassErrorResponse
	}

	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodCreatePermission, respClass), []stun.Setter{messageIntegrity}...)...)
}

func handleSendIndication(r Request, m *stun.Message) error {
	r.Log.Debugf("received SendIndication from %s", r.SrcAddr.String())
	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return fmt.Errorf("no allocation found for %v:%v", r.SrcAddr, r.Conn.LocalAddr())
	}

	dataAttr := proto.Data{}
	if err := dataAttr.GetFrom(m); err != nil {
		return err
	}

	peerAddress := proto.PeerAddress{}
	if err := peerAddress.GetFrom(m); err != nil {
		return err
	}

	msgDst := &net.UDPAddr{IP: peerAddress.IP, Port: peerAddress.Port}
	if perm := a.GetPermission(msgDst); perm == nil {
		return fmt.Errorf("unable to handle send-indication, no permission added: %v", msgDst)
	}

	l, err := a.RelaySocket.WriteTo(dataAttr, msgDst)
	if l != len(dataAttr) {
		return fmt.Errorf("packet write smaller than packet %d != %d (expected) err: %v", l, len(dataAttr), err)
	}
	return err
}

func handleChannelBindRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ChannelBindRequest from %s", r.SrcAddr.String())

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return fmt.Errorf("no allocation found for %v:%v", r.SrcAddr, r.Conn.LocalAddr())
	}

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodChannelBind)
	if !hasAuth {
		return err
	}

	var channel proto.ChannelNumber
	if err = channel.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	peerAddr := proto.PeerAddress{}
	if err = peerAddr.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	r.Log.Debugf("binding channel %d to %s",
		channel,
		fmt.Sprintf("%s:%d", peerAddr.IP.String(), peerAddr.Port))
	err = a.AddChannelBind(allocation.NewChannelBind(
		channel,
		&net.UDPAddr{IP: peerAddr.IP, Port: peerAddr.Port},
		r.Log,
	), r.ChannelBindTimeout)
	if err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodChannelBind, stun.ClassSuccessResponse), []stun.Setter{messageIntegrity}...)...)
}

// // Updated by https://tools.ietf.org/html/rfc6062#section-5.2
func handleConnectRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ConnectionRequest from %s", r.SrcAddr.String())

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnectionBind, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	// If the request is received on a TCP connection for which no
	// allocation exists, the server MUST return a 437 (Allocation Mismatch)
	// error.
	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		Protocol: allocation.TCP,
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
	})
	if a == nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeAllocMismatch})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("no allocation"), msg...)
	}

	// If the request does not contain an XOR-PEER-ADDRESS attribute, or if
	// such attribute is invalid, the server MUST return a 400 (Bad Request)
	// error.
	var peerAddr stun.XORMappedAddress
	if err := peerAddr.GetFromAs(m, stun.AttrXORPeerAddress); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("no XOR-PEER-ADDRESS"), badRequestMsg...)
	}

	// If the server is currently processing a Connect request for this
	// allocation with the same XOR-PEER-ADDRESS, it MUST return a 446
	// (Connection Already Exists) error.
	//
	// If the server has already successfully processed a Connect request
	// for this allocation with the same XOR-PEER-ADDRESS, and the resulting
	// client and peer data connections are either pending or active, it
	// MUST return a 446 (Connection Already Exists) error.
	if c := a.GetConnectionByAddr(peerAddr.String()); c != nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeConnAlreadyExists})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("connection already exists"), msg...)
	}

	// If the new connection is forbidden by local policy, the server MUST
	// reject the request with a 403 (Forbidden) error.
	//
	// Otherwise, the server MUST initiate an outgoing TCP connection.  The
	// local endpoint is the relayed transport address associated with the
	// allocation.  The remote endpoint is the one indicated by the XOR-
	// PEER-ADDRESS attribute.  If the connection attempt fails or times
	// out, the server MUST return a 447 (Connection Timeout or Failure)
	// error.  The timeout value MUST be at least 30 seconds.
	cid, err := r.AllocationManager.Connect(a, peerAddr.String())
	if err != nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeConnTimeoutOrFailure})
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("connection failure"), msg...)
	}

	// If the connection is successful, it is now called a peer data
	// connection.  The server MUST buffer any data received from the
	// client.  The server adjusts its advertised TCP receive window to
	// reflect the amount of empty buffer space.
	//
	// The server MUST include the CONNECTION-ID attribute in the Connect
	// success response.  The attribute's value MUST uniquely identify the
	// peer data connection.
	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse), ConnectionID(cid))...)
}

// // https://tools.ietf.org/html/rfc6062#section-5.4
func handleConnectionBindRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ConnectionBindRequest from %s", r.SrcAddr.String())

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnectionBind, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	// TODO: check if auth necessary?
	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodRefresh)
	if !hasAuth {

		return err
	}

	// If the client connection transport is not TCP or TLS, the server MUST
	// return a 400 (Bad Request) error.
	type conner interface{ Conn() net.Conn }
	stunconn, ok := r.Conn.(conner)
	if !ok {
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("must use TCP transport"), badRequestMsg...)
	}
	clientConn := stunconn.Conn()

	// If the request does not contain the CONNECTION-ID attribute, or if
	// this attribute does not refer to an existing pending connection, the
	// server MUST return a 400 (Bad Request) error.
	var cid ConnectionID
	if err := cid.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("no CONNECTION-ID"), badRequestMsg...)
	}

	peerConn := r.AllocationManager.BindConnection(uint32(cid))
	if peerConn == nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, fmt.Errorf("must use TCP transport"), badRequestMsg...)
	}

	// Otherwise, the client connection is now called a client data
	// connection.  Data received on it MUST be sent as-is to the associated
	// peer data connection.
	//
	// Data received on the associated peer data connection MUST be sent
	// as-is on this client data connection.  This includes data that was
	// received after the associated Connect or request was successfully
	// processed and before this ConnectionBind request was received.
	//
	// https://tools.ietf.org/html/rfc6062#section-5.5
	//
	// If the allocation associated with a data connection expires, the data
	// connection MUST be closed.
	//
	// When a client data connection is closed, the server MUST close the
	// corresponding peer data connection.
	//
	// When a peer data connection is closed, the server MUST close the
	// corresponding client data connection.
	go func() {
		io.Copy(clientConn, peerConn)
		clientConn.Close()
		peerConn.Close()
	}()
	go func() {
		io.Copy(peerConn, clientConn)
		clientConn.Close()
		peerConn.Close()
	}()

	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodConnectionBind, stun.ClassSuccessResponse), []stun.Setter{messageIntegrity}...)...)
}

func handleChannelData(r Request, c *proto.ChannelData) error {
	r.Log.Debugf("received ChannelData from %s", r.SrcAddr.String())

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return fmt.Errorf("no allocation found for %v:%v", r.SrcAddr, r.Conn.LocalAddr())
	}

	channel := a.GetChannelByNumber(c.Number)
	if channel == nil {
		return fmt.Errorf("no channel bind found for %x", uint16(c.Number))
	}

	l, err := a.RelaySocket.WriteTo(c.Data, channel.Peer)
	if err != nil {
		return fmt.Errorf("failed writing to socket: %s", err.Error())
	} else if l != len(c.Data) {
		return fmt.Errorf("packet write smaller than packet %d != %d (expected)", l, len(c.Data))
	}

	return nil
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
