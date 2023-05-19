// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/allocation"
	"github.com/pion/turn/v2/internal/ipnet"
	"github.com/pion/turn/v2/internal/proto"
	"github.com/pion/turn/v2/utils"
)

func parseAllocationPortAndToken(r Request, m *stun.Message, protocol proto.Protocol) (int, string, error) {
	requestedPort := 0
	reservationToken := ""

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})
	insufficientCapacityMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeInsufficientCapacity})

	switch protocol {
	case proto.ProtoUDP:
		// 4. The request may contain a DONT-FRAGMENT attribute.  If it does,
		//    but the server does not support sending UDP datagrams with the DF
		//    bit set to 1 (see Section 12), then the server treats the DONT-
		//    FRAGMENT attribute in the Allocate request as an unknown
		//    comprehension-required attribute.
		if m.Contains(stun.AttrDontFragment) {
			msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeUnknownAttribute}, &stun.UnknownAttributes{stun.AttrDontFragment})
			return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errNoDontFragmentSupport, msg...)
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
		if err := reservationTokenAttr.GetFrom(m); err == nil {
			var evenPort proto.EvenPort
			if err = evenPort.GetFrom(m); err == nil {
				return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errRequestWithReservationTokenAndEvenPort, badRequestMsg...)
			}
		}

		// 6. The server checks if the request contains an EVEN-PORT attribute.
		//    If yes, then the server checks that it can satisfy the request
		//    (i.e., can allocate a relayed transport address as described
		//    below).  If the server cannot satisfy the request, then the
		//    server rejects the request with a 508 (Insufficient Capacity)
		//    error.
		var evenPort proto.EvenPort
		if err := evenPort.GetFrom(m); err == nil {
			var randomPort int
			randomPort, err = r.AllocationManager.GetRandomEvenPort()
			if err != nil {
				return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, err, insufficientCapacityMsg...)
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
	case proto.ProtoTCP:
		//  2. If the client connection transport is not TCP or TLS, the server
		// 	MUST reject the request with a 400 (Bad Request) error.
		switch r.AllocationManager.Protocol {
		case allocation.TCP:
		case allocation.TLS:
		default:
			return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errUnsupportedClientProtocol, badRequestMsg...)
		}

		//  3. If the request contains the DONT-FRAGMENT, EVEN-PORT, or
		// 	RESERVATION-TOKEN attribute, the server MUST reject the request
		// 	with a 400 (Bad Request) error.
		if m.Contains(stun.AttrDontFragment) {
			return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errInvalidAttributeForTCPAllocation, badRequestMsg...)
		}

		if m.Contains(stun.AttrReservationToken) {
			return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errInvalidAttributeForTCPAllocation, badRequestMsg...)
		}

		if m.Contains(stun.AttrEvenPort) {
			return 0, "", buildAndSendErr(r.Conn, r.SrcAddr, errInvalidAttributeForTCPAllocation, badRequestMsg...)
		}

		//  4. A TCP relayed transport address MUST be allocated instead of a
		// 	UDP one.

		//  5. The RESERVATION-TOKEN attribute MUST NOT be present in the
		// 	success response.
	}

	return requestedPort, reservationToken, nil
}

// https://tools.ietf.org/html/rfc5766#section-6.2
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
		Protocol: r.AllocationManager.Protocol,
	}

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})
	insufficientCapacityMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeInsufficientCapacity})

	// 2. The server checks if the 5-tuple is currently in use by an
	//    existing allocation.  If yes, the server rejects the request with
	//    a 437 (Allocation Mismatch) error.
	if alloc := r.AllocationManager.GetAllocation(fiveTuple); alloc != nil {
		id, attrs := alloc.GetResponseCache()
		if id != m.TransactionID {
			msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeAllocMismatch})
			return buildAndSendErr(r.Conn, r.SrcAddr, errRelayAlreadyAllocatedForFiveTuple, msg...)
		}
		// A retry allocation
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse), append(attrs, messageIntegrity)...)
		return buildAndSend(r.Conn, r.SrcAddr, msg...)
	}

	// 3. The server checks if the request contains a REQUESTED-TRANSPORT
	//    attribute.  If the REQUESTED-TRANSPORT attribute is not included
	//    or is malformed, the server rejects the request with a 400 (Bad
	//    Request) error.  Otherwise, if the attribute is included but
	//    specifies a protocol other that UDP/TCP, the server rejects the
	//    request with a 442 (Unsupported Transport Protocol) error.
	var requestedTransport proto.RequestedTransport
	if err = requestedTransport.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	} else if requestedTransport.Protocol != proto.ProtoUDP && requestedTransport.Protocol != proto.ProtoTCP {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeUnsupportedTransProto})
		return buildAndSendErr(r.Conn, r.SrcAddr, errUnsupportedTransportProtocol, msg...)
	}

	requestedPort, reservationToken, err := parseAllocationPortAndToken(r, m, requestedTransport.Protocol)
	if err != nil {
		return err
	}

	lifetimeDuration := allocationLifeTime(m)
	a, err := r.AllocationManager.CreateAllocation(
		fiveTuple,
		r.Conn,
		requestedPort,
		lifetimeDuration,
		requestedTransport.Protocol,
	)
	if err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, insufficientCapacityMsg...)
	}

	// Once the allocation is created, the server replies with a success
	// response.
	// The success response contains:
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

	if reservationToken != "" {
		r.AllocationManager.CreateReservation(reservationToken, relayPort)
		responseAttrs = append(responseAttrs, proto.ReservationToken([]byte(reservationToken)))
	}

	msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse), append(responseAttrs, messageIntegrity)...)
	a.SetResponseCache(m.TransactionID, responseAttrs)
	return buildAndSend(r.Conn, r.SrcAddr, msg...)
}

// // https://tools.ietf.org/html/rfc5766#section-6.2
func handleConnectRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ConnectRequest from %s", r.SrcAddr.String())
	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	// 	If the request is received on a TCP connection for which no
	// 	allocation exists, the server MUST return a 437 (Allocation Mismatch)
	// 	error.

	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodConnect)
	if !hasAuth {
		return err
	}

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		Protocol: r.AllocationManager.Protocol,
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
	})
	if a == nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeAllocMismatch})
		return buildAndSendErr(r.Conn, r.SrcAddr, errNoAllocationFound, msg...)
	}

	// 	If the request does not contain an XOR-PEER-ADDRESS attribute, or if
	// 	such attribute is invalid, the server MUST return a 400 (Bad Request)
	// 	error.

	peerAddr := proto.PeerAddress{}
	if err = peerAddr.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, err, badRequestMsg...)
	}

	// 	If the server is currently processing a Connect request for this
	// 	allocation with the same XOR-PEER-ADDRESS, it MUST return a 446
	// 	(Connection Already Exists) error.
	// 	If the server has already successfully processed a Connect request
	// 	for this allocation with the same XOR-PEER-ADDRESS, and the resulting
	// 	client and peer data connections are either pending or active, it
	// 	MUST return a 446 (Connection Already Exists) error.

	dst := &net.TCPAddr{IP: peerAddr.IP, Port: peerAddr.Port}
	if c := a.GetConnectionByAddr(dst.String()); c != nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeConnAlreadyExists})
		return buildAndSendErr(r.Conn, r.SrcAddr, errConnectionAlreadyExists, msg...)
	}

	// 	If the new connection is forbidden by local policy, the server MUST
	// 	reject the request with a 403 (Forbidden) error.

	if err = r.AllocationManager.GrantPermission(r.SrcAddr, peerAddr.IP); err != nil {
		r.Log.Infof("permission denied for client %s to peer %s", r.SrcAddr.String(),
			peerAddr.IP.String())

		forbiddenMsg := buildMsg(m.TransactionID,
			stun.NewType(stun.MethodConnect, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeForbidden})
		return buildAndSendErr(r.Conn, r.SrcAddr, err, forbiddenMsg...)
	}

	// 	Otherwise, the server MUST initiate an outgoing TCP connection.  The
	// 	local endpoint is the relayed transport address associated with the
	// 	allocation.  The remote endpoint is the one indicated by the XOR-
	// 	PEER-ADDRESS attribute.  If the connection attempt fails or times
	// 	out, the server MUST return a 447 (Connection Timeout or Failure)
	// 	error.  The timeout value MUST be at least 30 seconds.
	cid, err := r.AllocationManager.Connect(a, dst)
	if err != nil {
		msg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeConnTimeoutOrFailure})
		return buildAndSendErr(r.Conn, r.SrcAddr, errConnectionFailure, msg...)
	}

	// 	If the connection is successful, it is now called a peer data
	// 	connection.  The server MUST buffer any data received from the
	// 	client.  The server adjusts its advertised TCP receive window to
	// 	reflect the amount of empty buffer space.

	// 	The server MUST include the CONNECTION-ID attribute in the Connect
	// 	success response.  The attribute's value MUST uniquely identify the
	// 	peer data connection.

	return buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse), proto.ConnectionID(cid), messageIntegrity)...)
}

func handleConnectionBindRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ConnectionBindRequest from %s", r.SrcAddr.String())

	badRequestMsg := buildMsg(m.TransactionID, stun.NewType(stun.MethodConnectionBind, stun.ClassErrorResponse), &stun.ErrorCodeAttribute{Code: stun.CodeBadRequest})

	// TODO: check if auth necessary?
	messageIntegrity, hasAuth, err := authenticateRequest(r, m, stun.MethodConnectionBind)
	if !hasAuth {
		return err
	}

	// If the client connection transport is not TCP or TLS, the server MUST
	// return a 400 (Bad Request) error.
	if r.AllocationManager.Protocol != allocation.TCP && r.AllocationManager.Protocol != allocation.TLS {
		return buildAndSendErr(r.Conn, r.SrcAddr, errConnectionBindRequireTCP, badRequestMsg...)
	}

	// If the request does not contain the CONNECTION-ID attribute, or if
	// this attribute does not refer to an existing pending connection, the
	// server MUST return a 400 (Bad Request) error.
	var cid proto.ConnectionID
	if err := cid.GetFrom(m); err != nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, errConnectionBindRequireID, badRequestMsg...)
	}

	peerConn := r.AllocationManager.BindConnection(proto.ConnectionID(cid))
	if peerConn == nil {
		return buildAndSendErr(r.Conn, r.SrcAddr, errConnectionNotFound, badRequestMsg...)
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
	clientConn := r.Conn.(*utils.STUNConn).Conn()
	err = buildAndSend(r.Conn, r.SrcAddr, buildMsg(m.TransactionID, stun.NewType(stun.MethodConnectionBind, stun.ClassSuccessResponse), messageIntegrity)...)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(clientConn, peerConn)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(peerConn, clientConn)
	}()

	wg.Wait()
	clientConn.Close()
	peerConn.Close()
	r.AllocationManager.DeleteConnection(cid)
	return nil
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
		Protocol: r.AllocationManager.Protocol,
	}

	if lifetimeDuration != 0 {
		a := r.AllocationManager.GetAllocation(fiveTuple)

		if a == nil {
			return fmt.Errorf("%w %v:%v", errNoAllocationFound, r.SrcAddr, r.Conn.LocalAddr())
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
		Protocol: r.AllocationManager.Protocol,
	})
	if a == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, r.SrcAddr, r.Conn.LocalAddr())
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

		if err := r.AllocationManager.GrantPermission(r.SrcAddr, peerAddress.IP); err != nil {
			r.Log.Infof("permission denied for client %s to peer %s", r.SrcAddr.String(),
				peerAddress.IP.String())
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
		Protocol: r.AllocationManager.Protocol,
	})
	if a == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, r.SrcAddr, r.Conn.LocalAddr())
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
		return fmt.Errorf("%w: %v", errNoPermission, msgDst)
	}

	l, err := a.RelaySocket.WriteTo(dataAttr, msgDst)
	if l != len(dataAttr) {
		return fmt.Errorf("%w %d != %d (expected) err: %v", errShortWrite, l, len(dataAttr), err) //nolint:errorlint
	}
	return err
}

func handleChannelBindRequest(r Request, m *stun.Message) error {
	r.Log.Debugf("received ChannelBindRequest from %s", r.SrcAddr.String())

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: r.AllocationManager.Protocol,
	})
	if a == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, r.SrcAddr, r.Conn.LocalAddr())
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

	if err = r.AllocationManager.GrantPermission(r.SrcAddr, peerAddr.IP); err != nil {
		r.Log.Infof("permission denied for client %s to peer %s", r.SrcAddr.String(),
			peerAddr.IP.String())

		unauthorizedRequestMsg := buildMsg(m.TransactionID,
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeUnauthorized})
		return buildAndSendErr(r.Conn, r.SrcAddr, err, unauthorizedRequestMsg...)
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

func handleChannelData(r Request, c *proto.ChannelData) error {
	r.Log.Debugf("received ChannelData from %s", r.SrcAddr.String())

	a := r.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  r.SrcAddr,
		DstAddr:  r.Conn.LocalAddr(),
		Protocol: r.AllocationManager.Protocol,
	})
	if a == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, r.SrcAddr, r.Conn.LocalAddr())
	}

	channel := a.GetChannelByNumber(c.Number)
	if channel == nil {
		return fmt.Errorf("%w %x", errNoSuchChannelBind, uint16(c.Number))
	}

	l, err := a.RelaySocket.WriteTo(c.Data, channel.Peer)
	if err != nil {
		return fmt.Errorf("%w: %s", errFailedWriteSocket, err.Error())
	} else if l != len(c.Data) {
		return fmt.Errorf("%w %d != %d (expected)", errShortWrite, l, len(c.Data))
	}

	return nil
}
