// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/pion/randutil"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/allocation"
	"github.com/pion/turn/v4/internal/ipnet"
	"github.com/pion/turn/v4/internal/proto"
)

const runesAlpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// See: https://tools.ietf.org/html/rfc5766#section-6.2
// .
func handleAllocateRequest(req Request, stunMsg *stun.Message) error { //nolint:cyclop,gocyclo,maintidx
	req.Log.Debugf("Received AllocateRequest from %s", req.SrcAddr)

	// 1. The server MUST require that the request be authenticated.  This
	//    authentication MUST be done using the long-term credential
	//    mechanism of [https://tools.ietf.org/html/rfc5389#section-10.2.2]
	//    unless the client and server agree to use another mechanism through
	//    some procedure outside the scope of this document.
	messageIntegrity, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodAllocate)
	if !hasAuth {
		return err
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	requestedPort := 0
	reservationToken := ""

	badRequestMsg := buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
		&stun.ErrorCodeAttribute{Code: stun.CodeBadRequest},
	)
	insufficientCapacityMsg := buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
		&stun.ErrorCodeAttribute{Code: stun.CodeInsufficientCapacity},
	)

	// 2. The server checks if the 5-tuple is currently in use by an
	//    existing allocation.  If yes, the server rejects the request with
	//    a 437 (Allocation Mismatch) error.
	if alloc := req.AllocationManager.GetAllocation(fiveTuple); alloc != nil {
		id, attrs := alloc.GetResponseCache()
		if id != stunMsg.TransactionID {
			msg := buildMsg(
				stunMsg.TransactionID,
				stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
				&stun.ErrorCodeAttribute{Code: stun.CodeAllocMismatch},
			)

			return buildAndSendErr(req.Conn, req.SrcAddr, errRelayAlreadyAllocatedForFiveTuple, msg...)
		}
		// A retry allocation
		msg := buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse),
			append(attrs, messageIntegrity)...,
		)

		return buildAndSend(req.Conn, req.SrcAddr, msg...)
	}

	// 3. The server checks if the request contains a REQUESTED-TRANSPORT
	//    attribute.  If the REQUESTED-TRANSPORT attribute is not included
	//    or is malformed, the server rejects the request with a 400 (Bad
	//    Request) error.  Otherwise, if the attribute is included but
	//    specifies a protocol other that UDP/TCP, the server rejects the
	//    request with a 442 (Unsupported Transport Protocol) error.
	var requestedTransport proto.RequestedTransport
	if err = requestedTransport.GetFrom(stunMsg); err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	} else if requestedTransport.Protocol != proto.ProtoUDP && requestedTransport.Protocol != proto.ProtoTCP {
		msg := buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeUnsupportedTransProto},
		)

		return buildAndSendErr(req.Conn, req.SrcAddr, errUnsupportedTransportProtocol, msg...)
	}

	// 4. The request may contain a DONT-FRAGMENT attribute.  If it does,
	//    but the server does not support sending UDP datagrams with the DF
	//    bit set to 1 (see Section 12), then the server treats the DONT-
	//    FRAGMENT attribute in the Allocate request as an unknown
	//    comprehension-required attribute.
	if stunMsg.Contains(stun.AttrDontFragment) {
		msg := buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeUnknownAttribute},
			&stun.UnknownAttributes{stun.AttrDontFragment},
		)

		return buildAndSendErr(req.Conn, req.SrcAddr, errNoDontFragmentSupport, msg...)
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
	if err = reservationTokenAttr.GetFrom(stunMsg); err == nil {
		var evenPort proto.EvenPort
		if err = evenPort.GetFrom(stunMsg); err == nil {
			return buildAndSendErr(req.Conn, req.SrcAddr, errRequestWithReservationTokenAndEvenPort, badRequestMsg...)
		}

		allocationPort, reservationFound := req.AllocationManager.GetReservation(string(reservationTokenAttr))
		if !reservationFound {
			return buildAndSendErr(req.Conn, req.SrcAddr, errNoAllocationFound, insufficientCapacityMsg...)
		}
		requestedPort = allocationPort + 1
	}

	// 6. The server checks if the request contains an EVEN-PORT attribute.
	//    If yes, then the server checks that it can satisfy the request
	//    (i.e., can allocate a relayed transport address as described
	//    below).  If the server cannot satisfy the request, then the
	//    server rejects the request with a 508 (Insufficient Capacity)
	//    error.
	var evenPort proto.EvenPort
	if err = evenPort.GetFrom(stunMsg); err == nil {
		var randomPort int
		randomPort, err = req.AllocationManager.GetRandomEvenPort()
		if err != nil {
			return buildAndSendErr(req.Conn, req.SrcAddr, err, insufficientCapacityMsg...)
		}
		requestedPort = randomPort
		reservationToken, err = randutil.GenerateCryptoRandomString(8, runesAlpha)
		if err != nil {
			return err
		}
	}

	// Parse realm (already checked in authenticateRequest)
	realmAttr := &stun.Realm{}
	_ = realmAttr.GetFrom(stunMsg)

	// RFC 6156: Parse REQUESTED-ADDRESS-FAMILY attribute if present.
	// If absent, default to IPv4 per RFC 6156 Section 4.1.1.
	var requestedFamily proto.RequestedAddressFamily
	if err = requestedFamily.GetFrom(stunMsg); err != nil {
		// Attribute not present or malformed - default to IPv4
		requestedFamily = proto.RequestedFamilyIPv4
	}

	// RFC 6156: Check if the requested address family is supported.
	// If not, reject with 440 (Address Family not Supported) error.
	if requestedFamily != proto.RequestedFamilyIPv4 && requestedFamily != proto.RequestedFamilyIPv6 {
		msg := buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeAddrFamilyNotSupported},
		)

		return buildAndSendErr(req.Conn, req.SrcAddr, errUnsupportedAddressFamily, msg...)
	}

	// RFC 6156: REQUESTED-ADDRESS-FAMILY and RESERVATION-TOKEN are mutually exclusive.
	if stunMsg.Contains(stun.AttrReservationToken) && stunMsg.Contains(stun.AttrRequestedAddressFamily) {
		return buildAndSendErr(req.Conn, req.SrcAddr, errRequestWithReservationTokenAndRequestedFamily, badRequestMsg...)
	}

	// 7. At any point, the server MAY choose to reject the request with a
	//    486 (Allocation Quota Reached) error if it feels the client is
	//    trying to exceed some locally defined allocation quota.  The
	//    server is free to define this allocation quota any way it wishes,
	//    but SHOULD define it based on the username used to authenticate
	//    the request, and not on the client's transport address.
	if req.QuotaHandler != nil && !req.QuotaHandler(username, realmAttr.String(), req.SrcAddr) {
		quotaReachedMsg := buildMsg(stunMsg.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeAllocQuotaReached})

		return buildAndSend(req.Conn, req.SrcAddr, quotaReachedMsg...)
	}

	// 8. Also at any point, the server MAY choose to reject the request
	//    with a 300 (Try Alternate) error if it wishes to redirect the
	//    client to a different server.  The use of this error code and
	//    attribute follow the specification in [RFC5389].
	lifetimeDuration := allocationLifeTime(req, stunMsg)
	alloc, err := req.AllocationManager.CreateAllocation(
		fiveTuple,
		req.Conn,
		requestedTransport.Protocol,
		requestedPort,
		lifetimeDuration,
		username,
		realmAttr.String(),
		requestedFamily,
	)
	if err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, insufficientCapacityMsg...)
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

	srcIP, srcPort, err := ipnet.AddrIPPort(req.SrcAddr)
	if err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	relayIP, relayPort, err := ipnet.AddrIPPort(alloc.RelayAddr)
	if err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
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
		req.AllocationManager.CreateReservation(reservationToken, relayPort)
		responseAttrs = append(responseAttrs, proto.ReservationToken([]byte(reservationToken)))
	}

	msg := buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodAllocate, stun.ClassSuccessResponse),
		append(responseAttrs, messageIntegrity)...,
	)
	alloc.SetResponseCache(stunMsg.TransactionID, responseAttrs)

	return buildAndSend(req.Conn, req.SrcAddr, msg...)
}

func handleRefreshRequest(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received RefreshRequest from %s", req.SrcAddr)

	messageIntegrity, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodRefresh)
	if !hasAuth {
		return err
	}

	lifetimeDuration := allocationLifeTime(req, stunMsg)
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}

	a := req.AllocationManager.GetAllocationForUsername(fiveTuple, username) //nolint:varnamelen
	if a == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	// RFC 6156: If REQUESTED-ADDRESS-FAMILY is present in Refresh request,
	// it must match the allocation's address family.
	var requestedFamily proto.RequestedAddressFamily
	if err = requestedFamily.GetFrom(stunMsg); err == nil {
		if requestedFamily != a.AddressFamily() {
			msg := buildMsg(
				stunMsg.TransactionID,
				stun.NewType(stun.MethodRefresh, stun.ClassErrorResponse),
				&stun.ErrorCodeAttribute{Code: stun.CodePeerAddrFamilyMismatch},
			)

			return buildAndSendErr(req.Conn, req.SrcAddr, errPeerAddressFamilyMismatch, msg...)
		}
	}

	if lifetimeDuration != 0 {
		a.Refresh(lifetimeDuration)
	} else {
		req.AllocationManager.DeleteAllocation(fiveTuple)
	}

	return buildAndSend(
		req.Conn,
		req.SrcAddr,
		buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodRefresh, stun.ClassSuccessResponse),
			[]stun.Setter{
				&proto.Lifetime{
					Duration: lifetimeDuration,
				},
				messageIntegrity,
			}...,
		)...,
	)
}

func handleCreatePermissionRequest(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received CreatePermission from %s", req.SrcAddr)

	messageIntegrity, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodCreatePermission)
	if !hasAuth {
		return err
	}

	alloc := req.AllocationManager.GetAllocationForUsername(&allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}, username)
	if alloc == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	addCount := 0

	if err := stunMsg.ForEach(stun.AttrXORPeerAddress, func(m *stun.Message) error {
		var peerAddress proto.PeerAddress
		if err := peerAddress.GetFrom(m); err != nil {
			return err
		}

		// RFC 6156: Peer address must match allocation's address family.
		if !ipMatchesFamily(peerAddress.IP, alloc.AddressFamily()) {
			req.Log.Infof("peer address family mismatch for client %s to peer %s", req.SrcAddr, peerAddress.IP)

			return errPeerAddressFamilyMismatch
		}

		if err := req.AllocationManager.GrantPermission(req.SrcAddr, peerAddress.IP); err != nil {
			req.Log.Infof("permission denied for client %s to peer %s", req.SrcAddr, peerAddress.IP)

			return err
		}

		req.Log.Debugf("Adding permission for %s", fmt.Sprintf("%s:%d",
			peerAddress.IP, peerAddress.Port))

		alloc.AddPermission(allocation.NewPermission(
			&net.UDPAddr{
				IP:   peerAddress.IP,
				Port: peerAddress.Port,
			},
			req.Log,
			req.PermissionTimeout,
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

	return buildAndSend(
		req.Conn,
		req.SrcAddr,
		buildMsg(stunMsg.TransactionID, stun.NewType(stun.MethodCreatePermission, respClass),
			[]stun.Setter{messageIntegrity}...)...,
	)
}

func handleSendIndication(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received SendIndication from %s", req.SrcAddr)
	alloc := req.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if alloc == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	dataAttr := proto.Data{}
	if err := dataAttr.GetFrom(stunMsg); err != nil {
		return err
	}

	peerAddress := proto.PeerAddress{}
	if err := peerAddress.GetFrom(stunMsg); err != nil {
		return err
	}

	msgDst := &net.UDPAddr{IP: peerAddress.IP, Port: peerAddress.Port}
	if perm := alloc.GetPermission(msgDst); perm == nil {
		return fmt.Errorf("%w: %v", errNoPermission, msgDst)
	}

	l, err := alloc.WriteTo(dataAttr, msgDst)
	if err != nil {
		return fmt.Errorf("%w: %s", errFailedWriteSocket, err.Error())
	} else if l != len(dataAttr) {
		return fmt.Errorf("%w %d != %d (expected)", errShortWrite, l, len(dataAttr))
	}

	return err
}

func handleChannelBindRequest(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received ChannelBindRequest from %s", req.SrcAddr)

	badRequestMsg := buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
		&stun.ErrorCodeAttribute{Code: stun.CodeBadRequest},
	)

	messageIntegrity, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodChannelBind)
	if !hasAuth {
		return err
	}

	alloc := req.AllocationManager.GetAllocationForUsername(&allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}, username)
	if alloc == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	var channel proto.ChannelNumber
	if err = channel.GetFrom(stunMsg); err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	peerAddr := proto.PeerAddress{}
	if err = peerAddr.GetFrom(stunMsg); err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	// RFC 6156: Peer address must match allocation's address family.
	if !ipMatchesFamily(peerAddr.IP, alloc.AddressFamily()) {
		req.Log.Infof("peer address family mismatch for client %s to peer %s", req.SrcAddr, peerAddr.IP)

		peerAddrFamilyMismatchMsg := buildMsg(stunMsg.TransactionID,
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodePeerAddrFamilyMismatch})

		return buildAndSendErr(req.Conn, req.SrcAddr, errPeerAddressFamilyMismatch, peerAddrFamilyMismatchMsg...)
	}

	if err = req.AllocationManager.GrantPermission(req.SrcAddr, peerAddr.IP); err != nil {
		req.Log.Infof("permission denied for client %s to peer %s", req.SrcAddr, peerAddr.IP)

		unauthorizedRequestMsg := buildMsg(stunMsg.TransactionID,
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeUnauthorized})

		return buildAndSendErr(req.Conn, req.SrcAddr, err, unauthorizedRequestMsg...)
	}

	req.Log.Debugf("Binding channel %d to %s", channel, peerAddr)
	err = alloc.AddChannelBind(allocation.NewChannelBind(
		channel,
		&net.UDPAddr{IP: peerAddr.IP, Port: peerAddr.Port},
		req.Log,
	), req.ChannelBindTimeout, req.PermissionTimeout)
	if err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	return buildAndSend(
		req.Conn,
		req.SrcAddr,
		buildMsg(stunMsg.TransactionID, stun.NewType(stun.MethodChannelBind, stun.ClassSuccessResponse),
			[]stun.Setter{messageIntegrity}...)...,
	)
}

func handleChannelData(req Request, channelData *proto.ChannelData) error {
	req.Log.Debugf("Received ChannelData from %s", req.SrcAddr)

	alloc := req.AllocationManager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if alloc == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	channel := alloc.GetChannelByNumber(channelData.Number)
	if channel == nil {
		return fmt.Errorf("%w %x", errNoSuchChannelBind, uint16(channelData.Number))
	}

	l, err := alloc.WriteTo(channelData.Data, channel.Peer)
	if err != nil {
		return fmt.Errorf("%w: %s", errFailedWriteSocket, err.Error())
	} else if l != len(channelData.Data) {
		return fmt.Errorf("%w %d != %d (expected)", errShortWrite, l, len(channelData.Data))
	}

	return nil
}

func handleConnectRequest(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received Connect from %s", req.SrcAddr)

	messageIntegrity, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodConnect)
	if !hasAuth {
		return err
	}

	alloc := req.AllocationManager.GetAllocationForUsername(&allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}, username)
	if alloc == nil {
		return fmt.Errorf("%w %v:%v", errNoAllocationFound, req.SrcAddr, req.Conn.LocalAddr())
	}

	// If the request does not contain an XOR-PEER-ADDRESS attribute, or if
	// such attribute is invalid, the server MUST return a 400 (Bad Request)
	// error.
	var peerAddr proto.PeerAddress
	if err = peerAddr.GetFrom(stunMsg); err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, buildMsg(
			stunMsg.TransactionID,
			stun.NewType(stun.MethodConnect, stun.ClassErrorResponse),
			&stun.ErrorCodeAttribute{Code: stun.CodeBadRequest},
		)...)
	}

	connectionID, err := req.AllocationManager.CreateTCPConnection(alloc, peerAddr)
	if err != nil {
		// If the server is currently processing a Connect request for this
		// allocation with the same XOR-PEER-ADDRESS, it MUST return a 446
		// (Connection Already Exists) error.

		// If the server has already successfully processed a Connect request
		// for this allocation with the same XOR-PEER-ADDRESS, and the resulting
		// client and peer data connections are either pending or active, it
		// MUST return a 446 (Connection Already Exists) error.
		if errors.Is(err, allocation.ErrDupeTCPConnection) {
			return buildAndSendErr(req.Conn, req.SrcAddr, err, buildMsg(
				stunMsg.TransactionID,
				stun.NewType(stun.MethodConnect, stun.ClassErrorResponse),
				&stun.ErrorCodeAttribute{Code: stun.CodeConnAlreadyExists},
			)...)
		}

		// Otherwise, the server MUST initiate an outgoing TCP connection.  The
		// local endpoint is the relayed transport address associated with the
		// allocation.  The remote endpoint is the one indicated by the XOR-
		// PEER-ADDRESS attribute.  If the connection attempt fails or times
		// out, the server MUST return a 447 (Connection Timeout or Failure)
		// error.  The timeout value MUST be at least 30 seconds.
		if errors.Is(err, allocation.ErrTCPConnectionTimeoutOrFailure) {
			return buildAndSendErr(req.Conn, req.SrcAddr, err, buildMsg(
				stunMsg.TransactionID,
				stun.NewType(stun.MethodConnect, stun.ClassErrorResponse),
				&stun.ErrorCodeAttribute{Code: stun.CodeConnTimeoutOrFailure},
			)...)
		}

		return err
	}

	// The server MUST include the CONNECTION-ID attribute in the Connect
	// success response.  The attribute's value MUST uniquely identify the
	// peer data connection.
	return buildAndSend(req.Conn, req.SrcAddr, buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse),
		connectionID,
		messageIntegrity,
	)...)
}

func handleConnectionBindRequest(req Request, stunMsg *stun.Message) error {
	req.Log.Debugf("Received ConnectBind from %s", req.SrcAddr)
	badRequestMsg := buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodConnectionBind, stun.ClassErrorResponse),
		&stun.ErrorCodeAttribute{Code: stun.CodeBadRequest},
	)

	_, hasAuth, username, err := authenticateRequest(req, stunMsg, stun.MethodConnectionBind)
	if !hasAuth {
		return err
	}

	var connectionID proto.ConnectionID
	if err = connectionID.GetFrom(stunMsg); err != nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	// Authentication of the client by the server MUST use the same method
	// and credentials as for the control connection.
	//
	// GetTCPConnection asserts that userName used for auth is same as allocation
	tcpConn := req.AllocationManager.GetTCPConnection(username, connectionID)
	if tcpConn == nil {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	stunConn, ok := req.Conn.(*proto.STUNConn)
	if !ok {
		return buildAndSendErr(req.Conn, req.SrcAddr, err, badRequestMsg...)
	}

	if err = buildAndSend(req.Conn, req.SrcAddr, buildMsg(
		stunMsg.TransactionID,
		stun.NewType(stun.MethodConnectionBind, stun.ClassSuccessResponse),
		connectionID,
	)...); err != nil {
		return err
	}

	copyCompleteCtx, copyCompleteCancel := context.WithCancel(context.Background())
	go func() {
		if _, ioErr := io.Copy(tcpConn, stunConn.Conn()); ioErr != nil {
			req.Log.Debugf("Exit tcpConn->stunConn read loop on error: %s", ioErr)
		}
		copyCompleteCancel()
	}()

	go func() {
		if _, ioErr := io.Copy(stunConn.Conn(), tcpConn); ioErr != nil {
			req.Log.Debugf("Exit stunConn->tcpConn read loop on error: %s", ioErr)
		}
		copyCompleteCancel()
	}()

	// When either side has failed close both
	<-copyCompleteCtx.Done()
	if err = tcpConn.Close(); err != nil {
		req.Log.Debugf("Close tcpConn error: %s", err)
	}
	if err = stunConn.Conn().Close(); err != nil {
		req.Log.Debugf("Close stunConn error: %s", err)
	}

	req.AllocationManager.RemoveTCPConnection(connectionID)

	return nil
}

// ipMatchesFamily checks if an IP address matches the given address family.
func ipMatchesFamily(ip net.IP, family proto.RequestedAddressFamily) bool {
	if family == proto.RequestedFamilyIPv4 {
		return ip.To4() != nil
	}
	if family == proto.RequestedFamilyIPv6 {
		return ip.To4() == nil && ip.To16() != nil
	}

	return false
}
