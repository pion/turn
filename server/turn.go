package server

import (
	"net"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/stun"
	"github.com/pkg/errors"

	"github.com/pion/turn/internal/allocation"
	"github.com/pion/turn/internal/ipnet"
)

const maximumLifetime = time.Hour // https://tools.ietf.org/html/rfc5766#section-6.2 defines 3600 seconds recommendation

// https://tools.ietf.org/html/rfc5766#section-6.2
// caller must hold the mutex
func (s *Server) handleAllocateRequest(ctx *context) error {
	// 1. The server MUST require that the request be authenticated.  This
	//    authentication MUST be done using the long-term credential
	//    mechanism of [https://tools.ietf.org/html/rfc5389#section-10.2.2]
	//    unless the client and server agree to use another mechanism through
	//    some procedure outside the scope of this document.
	if err := ctx.authenticate(); err != nil {
		return err
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  ctx.srcAddr,
		DstAddr:  ctx.conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	requestedPort := 0
	reservationToken := ""

	// 2. The server checks if the 5-tuple is currently in use by an
	//    existing allocation.  If yes, the server rejects the request with
	//    a 437 (Allocation Mismatch) error.
	if alloc := s.manager.GetAllocation(fiveTuple); alloc != nil {
		return ctx.respondWithError(errors.Errorf("Relay already allocated for 5-TUPLE"), stun.CodeAllocMismatch)
	}

	// 3. The server checks if the request contains a REQUESTED-TRANSPORT
	//    attribute.  If the REQUESTED-TRANSPORT attribute is not included
	//    or is malformed, the server rejects the request with a 400 (Bad
	//    Request) error.  Otherwise, if the attribute is included but
	//    specifies a protocol other that UDP, the server rejects the
	//    request with a 442 (Unsupported Transport Protocol) error.
	var requestedTransport turn.RequestedTransport
	if err := requestedTransport.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}
	if requestedTransport.Protocol != turn.ProtoUDP {
		return ctx.respondWithError(errors.New(""), stun.CodeUnsupportedTransProto)
	}

	// 4. The request may contain a DONT-FRAGMENT attribute.  If it does,
	//    but the server does not support sending UDP datagrams with the DF
	//    bit set to 1 (see Section 12), then the server treats the DONT-
	//    FRAGMENT attribute in the Allocate request as an unknown
	//    comprehension-required attribute.

	if ctx.msg.Contains(stun.AttrDontFragment) {
		return ctx.respondWithError(errors.Errorf("no support for DONT-FRAGMENT"), stun.CodeUnknownAttribute, &stun.UnknownAttributes{stun.AttrDontFragment})
	}

	// 5.  The server checks if the request contains a RESERVATION-TOKEN
	//     attribute.  If yes, and the request also contains an EVEN-PORT
	//     attribute, then the server rejects the request with a 400 (Bad
	//     Request) error.  Otherwise, it checks to see if the token is
	//     valid (i.e., the token is in range and has not expired and the
	//     corresponding relayed transport address is still available).  If
	//     the token is not valid for some reason, the server rejects the
	//     request with a 508 (Insufficient Capacity) error.
	var reservationTokenAttr turn.ReservationToken
	if err := reservationTokenAttr.GetFrom(ctx.msg); err == nil {
		var evenPort turn.EvenPort
		if err = evenPort.GetFrom(ctx.msg); err == nil {
			return ctx.respondWithError(errors.Errorf("Request must not contain RESERVATION-TOKEN and EVEN-PORT"), stun.CodeBadRequest)
		}

		allocationPort, reservationFound := s.reservationManager.GetReservation(string(reservationTokenAttr))
		if !reservationFound {
			return ctx.respondWithError(errors.Errorf("No reservation found with token %s", string(reservationTokenAttr)), stun.CodeBadRequest)
		}
		requestedPort = allocationPort + 1
	}

	// 6. The server checks if the request contains an EVEN-PORT attribute.
	//    If yes, then the server checks that it can satisfy the request
	//    (i.e., can allocate a relayed transport address as described
	//    below).  If the server cannot satisfy the request, then the
	//    server rejects the request with a 508 (Insufficient Capacity)
	//    error.
	var evenPort turn.EvenPort
	if err := evenPort.GetFrom(ctx.msg); err == nil {
		randomPort := 0
		randomPort, err = allocation.GetRandomEvenPort()
		if err != nil {
			return ctx.respondWithError(err, stun.CodeInsufficientCapacity)
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
	// Check current usage vs redis usage of other servers
	// if bad, redirect { stun.AttrErrorCode, 300 }
	lifetimeDuration := allocationLifeTime(ctx.msg)
	a, err := s.manager.CreateAllocation(fiveTuple, ctx.conn, requestedPort, lifetimeDuration)
	if err != nil {
		return ctx.respondWithError(err, stun.CodeInsufficientCapacity)
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

	srcIP, srcPort, err := ipnet.AddrIPPort(ctx.srcAddr)
	if err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	_, relayPort, err := ipnet.AddrIPPort(a.RelayAddr)
	if err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	dstIP, _, err := ipnet.AddrIPPort(ctx.conn.LocalAddr())
	if err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	responseAttrs := []stun.Setter{
		&turn.RelayedAddress{
			IP:   dstIP,
			Port: relayPort,
		},
		&turn.Lifetime{
			Duration: lifetimeDuration,
		},
		&stun.XORMappedAddress{
			IP:   srcIP,
			Port: srcPort,
		},
	}

	if reservationToken != "" {
		s.reservationManager.CreateReservation(reservationToken, relayPort)
		responseAttrs = append(responseAttrs, turn.ReservationToken([]byte(reservationToken)))
	}

	return ctx.respond(responseAttrs...)
}

// caller must hold the mutex
func (s *Server) handleRefreshRequest(ctx *context) error {
	if err := ctx.authenticate(); err != nil {
		return err
	}

	a := s.manager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  ctx.srcAddr,
		DstAddr:  ctx.conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return errors.Errorf("No allocation found for %v:%v", ctx.srcAddr, ctx.conn.LocalAddr())
	}

	lifetimeDuration := allocationLifeTime(ctx.msg)
	a.Refresh(lifetimeDuration)

	return ctx.respond(&turn.Lifetime{
		Duration: lifetimeDuration,
	})
}

// caller must hold the mutex
func (s *Server) handleCreatePermissionRequest(ctx *context) error {
	a := s.manager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  ctx.srcAddr,
		DstAddr:  ctx.conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return errors.Errorf("No allocation found for %v:%v", ctx.srcAddr, ctx.conn.LocalAddr())
	}

	if err := ctx.authenticate(); err != nil {
		return err
	}

	addCount := 0

	if err := ctx.msg.ForEach(stun.AttrXORPeerAddress, func(m *stun.Message) error {
		var peerAddress turn.PeerAddress
		if err := peerAddress.GetFrom(m); err != nil {
			return err
		}

		a.AddPermission(allocation.NewPermission(
			&net.UDPAddr{
				IP:   peerAddress.IP,
				Port: peerAddress.Port,
			},
			s.log,
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

	return ctx.buildAndSend(respClass)
}

// caller must hold the mutex
func (s *Server) handleSendIndication(ctx *context) error {
	a := s.manager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  ctx.srcAddr,
		DstAddr:  ctx.conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return errors.Errorf("No allocation found for %v:%v", ctx.srcAddr, ctx.conn.LocalAddr())
	}

	dataAttr := turn.Data{}
	if err := dataAttr.GetFrom(ctx.msg); err != nil {
		return err
	}

	peerAddress := turn.PeerAddress{}
	if err := peerAddress.GetFrom(ctx.msg); err != nil {
		return err
	}

	msgDst := &net.UDPAddr{IP: peerAddress.IP, Port: peerAddress.Port}
	if perm := a.GetPermission(msgDst.String()); perm == nil {
		return errors.Errorf("Unable to handle send-indication, no permission added: %v", msgDst)
	}

	l, err := a.RelaySocket.WriteTo(dataAttr, msgDst)
	if l != len(dataAttr) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected) err: %v", l, len(dataAttr), err)
	}
	return err
}

// caller must hold the mutex
func (s *Server) handleChannelBindRequest(ctx *context) error {
	a := s.manager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  ctx.srcAddr,
		DstAddr:  ctx.conn.LocalAddr(),
		Protocol: allocation.UDP,
	})
	if a == nil {
		return errors.Errorf("No allocation found for %v:%v", ctx.srcAddr, ctx.conn.LocalAddr())
	}

	if err := ctx.authenticate(); err != nil {
		return err
	}

	var channel turn.ChannelNumber
	if err := channel.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	peerAddr := turn.PeerAddress{}
	if err := peerAddr.GetFrom(ctx.msg); err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	err := a.AddChannelBind(allocation.NewChannelBind(
		channel,
		&net.UDPAddr{IP: peerAddr.IP, Port: peerAddr.Port},
		s.log,
	), s.channelBindTimeout)
	if err != nil {
		return ctx.respondWithError(err, stun.CodeBadRequest)
	}

	return ctx.respond()
}

func (s *Server) handleChannelData(conn net.PacketConn, srcAddr net.Addr, c *turn.ChannelData) error {
	dstAddr := conn.LocalAddr()
	a := s.manager.GetAllocation(&allocation.FiveTuple{
		SrcAddr:  srcAddr,
		DstAddr:  dstAddr,
		Protocol: allocation.UDP,
	})
	if a == nil {
		return errors.Errorf("No allocation found for %v:%v", srcAddr, dstAddr)
	}

	channel := a.GetChannelByID(c.Number)
	if channel == nil {
		return errors.Errorf("No channel bind found for %x \n", uint16(c.Number))
	}

	l, err := a.RelaySocket.WriteTo(c.Data, channel.Peer)
	if err != nil {
		return errors.Wrap(err, "failed writing to socket")
	}

	if l != len(c.Data) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected)", l, len(c.Data))
	}

	return nil
}

func allocationLifeTime(m *stun.Message) time.Duration {
	lifetimeDuration := turn.DefaultLifetime

	var lifetime turn.Lifetime
	if err := lifetime.GetFrom(m); err == nil {
		if lifetime.Duration < maximumLifetime {
			lifetimeDuration = lifetime.Duration
		}
	}

	return lifetimeDuration
}
