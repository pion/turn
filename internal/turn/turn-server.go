package turnServer

import (
	"crypto/md5"
	"os"
	"strings"

	"github.com/pions/pkg/stun"
	"github.com/pkg/errors"

	"gitlab.com/pions/pion/turn/internal/relay"
)

const (
	maximumLifetime = uint32(3600) // https://tools.ietf.org/html/rfc5766#section-6.2 defines 3600 recommendation
	defaultLifetime = uint32(600)  // https://tools.ietf.org/html/rfc5766#section-2.2 defines 600 recommendation
)

func buildTransactionId() []byte {
	transactionID := []byte(randSeq(16))
	transactionID[0] = 33
	transactionID[1] = 18
	transactionID[2] = 164
	transactionID[3] = 66
	return transactionID
}

type CurriedSend func(class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error

func authenticateRequest(curriedSend CurriedSend, m *stun.Message, callingMethod stun.Method) (*stun.MessageIntegrity, string, error) {
	if _, integrityFound := m.GetOneAttribute(stun.AttrMessageIntegrity); !integrityFound {
		return nil, "", curriedSend(stun.ClassErrorResponse, callingMethod, m.TransactionID,
			&stun.Err401Unauthorized,
			&stun.Nonce{buildNonce()},
			&stun.Realm{os.Getenv("FQDN")},
		)
	}
	var err error
	nonceAttr := &stun.Nonce{}
	usernameAttr := &stun.Username{}
	realmAttr := &stun.Realm{}

	if usernameRawAttr, usernameFound := m.GetOneAttribute(stun.AttrUsername); true {
		if usernameFound {
			err = usernameAttr.Unpack(m, usernameRawAttr)
		} else {
			err = errors.Errorf("Integrity found, but missing username")
		}
	}

	if realmRawAttr, realmFound := m.GetOneAttribute(stun.AttrRealm); true {
		if realmFound {
			err = realmAttr.Unpack(m, realmRawAttr)
		} else {
			err = errors.Errorf("Integrity found, but missing realm")
		}
	}

	if nonceRawAttr, nonceFound := m.GetOneAttribute(stun.AttrNonce); true {
		if nonceFound {
			err = nonceAttr.Unpack(m, nonceRawAttr)
		} else {
			err = errors.Errorf("Integrity found, but missing nonce")
		}
	}

	if err != nil {
		if sendErr := curriedSend(stun.ClassErrorResponse, callingMethod, m.TransactionID,
			&stun.Err400BadRequest,
		); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return nil, "", err
	}
	return &stun.MessageIntegrity{
		Key: md5.Sum([]byte(usernameAttr.Username + ":" + realmAttr.Realm + ":" + "password")),
	}, usernameAttr.Username, nil
}

func assertDontFragment(curriedSend CurriedSend, m *stun.Message, callingMethod stun.Method, messageIntegrity *stun.MessageIntegrity) error {
	if _, ok := m.GetOneAttribute(stun.AttrDontFragment); ok {
		err := errors.Errorf("no support for DONT-FRAGMENT")
		if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
			&stun.Err420UnknownAttributes,
			&stun.UnknownAttributes{[]stun.AttrType{stun.AttrDontFragment}},
			messageIntegrity,
		); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return err
	}
	return nil
}

// https://tools.ietf.org/html/rfc5766#section-6.2
func (s *Server) handleAllocateRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	curriedSend := func(class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
		return stun.BuildAndSend(s.connection, srcAddr, class, method, transactionID, attrs...)
	}

	// 1. The server MUST require that the request be authenticated.  This
	//    authentication MUST be done using the long-term credential
	//    mechanism of [https://tools.ietf.org/html/rfc5389#section-10.2.2]
	//    unless the client and server agree to use another mechanism through
	//    some procedure outside the scope of this document.
	messageIntegrity, username, err := authenticateRequest(curriedSend, m, stun.MethodAllocate)
	if err != nil {
		return err
	} else if messageIntegrity == nil {
		return nil
	}

	fiveTuple := &relayServer.FiveTuple{
		SrcAddr:  srcAddr,
		DstAddr:  dstAddr,
		Protocol: relayServer.UDP,
	}

	// 2. The server checks if the 5-tuple is currently in use by an
	//    existing allocation.  If yes, the server rejects the request with
	//    a 437 (Allocation Mismatch) error.
	if relayServer.Fulfilled(fiveTuple) {
		err := errors.Errorf("Relay already allocated for 5-TUPLE")
		if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
			&stun.Err437AllocationMismatch,
			messageIntegrity,
		); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return err
	}

	// 3. The server checks if the request contains a REQUESTED-TRANSPORT
	//    attribute.  If the REQUESTED-TRANSPORT attribute is not included
	//    or is malformed, the server rejects the request with a 400 (Bad
	//    Request) error.  Otherwise, if the attribute is included but
	//    specifies a protocol other that UDP, the server rejects the
	//    request with a 442 (Unsupported Transport Protocol) error.
	if requestedTransportRawAttr, ok := m.GetOneAttribute(stun.AttrRequestedTransport); true {
		if !ok {
			err := errors.Errorf("Allocation request missing REQUESTED-TRANSPORT")
			if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
				&stun.Err400BadRequest,
				messageIntegrity,
			); sendErr != nil {
				err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
			}
			return err
		}

		requestedTransportAttr := &stun.RequestedTransport{}
		if err := requestedTransportAttr.Unpack(m, requestedTransportRawAttr); err != nil {
			if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
				&stun.Err400BadRequest,
				messageIntegrity,
			); sendErr != nil {
				err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
			}
			return err
		}
	}

	// 4. The request may contain a DONT-FRAGMENT attribute.  If it does,
	//    but the server does not support sending UDP datagrams with the DF
	//    bit set to 1 (see Section 12), then the server treats the DONT-
	//    FRAGMENT attribute in the Allocate request as an unknown
	//    comprehension-required attribute.
	if err := assertDontFragment(curriedSend, m, stun.MethodAllocate, messageIntegrity); err != nil {
		return err
	}

	// 5.  The server checks if the request contains a RESERVATION-TOKEN
	//     attribute.  If yes, and the request also contains an EVEN-PORT
	//     attribute, then the server rejects the request with a 400 (Bad
	//     Request) error.  Otherwise, it checks to see if the token is
	//     valid (i.e., the token is in range and has not expired and the
	//     corresponding relayed transport address is still available).  If
	//     the token is not valid for some reason, the server rejects the
	//     request with a 508 (Insufficient Capacity) error.
	if _, ok := m.GetOneAttribute(stun.AttrReservationToken); ok {
		if _, ok := m.GetOneAttribute(stun.AttrEvenPort); ok {
			err := errors.Errorf("no support for DONT-FRAGMENT")
			if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
				&stun.Err400BadRequest,
				messageIntegrity,
			); sendErr != nil {
				err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
			}
			return err
		}

		panic("TODO check reservation validity")
	}

	// 6. The server checks if the request contains an EVEN-PORT attribute.
	//    If yes, then the server checks that it can satisfy the request
	//    (i.e., can allocate a relayed transport address as described
	//    below).  If the server cannot satisfy the request, then the
	//    server rejects the request with a 508 (Insufficient Capacity)
	//    error.
	if _, ok := m.GetOneAttribute(stun.AttrEvenPort); ok {
		err := errors.Errorf("no support for EVEN-PORT")
		if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
			&stun.Err508InsufficentCapacity,
			messageIntegrity,
		); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return err
	}

	// 7. At any point, the server MAY choose to reject the request with a
	//    486 (Allocation Quota Reached) error if it feels the client is
	//    trying to exceed some locally defined allocation quota.  The
	//    server is free to define this allocation quota any way it wishes,
	//    but SHOULD define it based on the username used to authenticate
	//    the request, and not on the client's transport address.
	// Check redis for username allocs of transports
	// if err return { stun.AttrErrorCode, 486 }

	// 8. Also at any point, the server MAY choose to reject the request
	//    with a 300 (Try Alternate) error if it wishes to redirect the
	//    client to a different server.  The use of this error code and
	//    attribute follow the specification in [RFC5389].
	// Check current usage vs redis usage of other servers
	// if bad, redirect { stun.AttrErrorCode, 300 }

	lifetimeDuration := defaultLifetime
	if lifetimeRawAttr, ok := m.GetOneAttribute(stun.AttrLifetime); ok {
		lifetimeAttr := stun.Lifetime{}
		if err := lifetimeAttr.Unpack(m, lifetimeRawAttr); err == nil {
			lifetimeDuration = min(lifetimeAttr.Duration, maximumLifetime)
		}
	}

	reservationToken := randSeq(8)
	relayPort, err := relayServer.Start(fiveTuple, reservationToken, lifetimeDuration, username)
	if err != nil {
		if sendErr := curriedSend(stun.ClassErrorResponse, stun.MethodAllocate, m.TransactionID,
			&stun.Err508InsufficentCapacity,
			messageIntegrity,
		); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return err
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
	return curriedSend(stun.ClassSuccessResponse, stun.MethodAllocate, m.TransactionID,
		&stun.XorRelayedAddress{
			XorAddress: stun.XorAddress{
				IP:   dstAddr.IP,
				Port: relayPort,
			},
		},
		&stun.Lifetime{
			Duration: lifetimeDuration,
		},
		&stun.ReservationToken{
			ReservationToken: reservationToken,
		},
		&stun.XorMappedAddress{
			XorAddress: stun.XorAddress{
				IP:   srcAddr.IP,
				Port: srcAddr.Port,
			},
		},
		messageIntegrity,
	)
}

func (s *Server) handleRefreshRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	curriedSend := func(class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
		return stun.BuildAndSend(s.connection, srcAddr, class, method, transactionID, attrs...)
	}
	messageIntegrity, _, err := authenticateRequest(curriedSend, m, stun.MethodCreatePermission)
	if err != nil {
		return err
	}

	return curriedSend(stun.ClassSuccessResponse, stun.MethodRefresh, m.TransactionID,
		messageIntegrity,
	)
}

func (s *Server) handleCreatePermissionRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	curriedSend := func(class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
		return stun.BuildAndSend(s.connection, srcAddr, class, method, transactionID, attrs...)
	}

	messageIntegrity, _, err := authenticateRequest(curriedSend, m, stun.MethodCreatePermission)
	if err != nil {
		return err
	}

	fiveTuple := &relayServer.FiveTuple{
		SrcAddr:  srcAddr,
		DstAddr:  dstAddr,
		Protocol: relayServer.UDP,
	}

	addCount := 0
	if xpas, ok := m.GetAllAttributes(stun.AttrXORPeerAddress); ok {
		for _, a := range xpas {
			peerAddress := stun.XorPeerAddress{}
			if err := peerAddress.Unpack(m, a); err == nil {
				if err := relayServer.AddPermission(fiveTuple, &relayServer.Permission{
					IP:           peerAddress.XorAddress.IP,
					TimeToExpiry: 300,
				}); err == nil {
					addCount++
				}
			}
		}
	}
	respClass := stun.ClassSuccessResponse
	if addCount == 0 {
		respClass = stun.ClassErrorResponse
	}

	return curriedSend(respClass, stun.MethodCreatePermission, m.TransactionID,
		messageIntegrity)
}

func (s *Server) handleSendIndication(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	dataAttr := stun.Data{}
	xorPeerAddress := stun.XorPeerAddress{}

	dataRawAttr, ok := m.GetOneAttribute(stun.AttrData)
	if !ok {
		return nil
	}
	if err := dataAttr.Unpack(m, dataRawAttr); err != nil {
		return err
	}

	xorPeerAddressRawAttr, ok := m.GetOneAttribute(stun.AttrXORPeerAddress)
	if !ok {
		return nil
	}
	if err := xorPeerAddress.Unpack(m, xorPeerAddressRawAttr); err != nil {
		return err
	}

	peerRelayAddr, err := relayServer.GetSrcForRelay(&stun.TransportAddr{IP: xorPeerAddress.XorAddress.IP, Port: xorPeerAddress.XorAddress.Port})
	if err != nil {
		return err
	}

	peerRelayPort, err := relayServer.GetRelayForSrc(srcAddr)
	if err != nil {
		return err
	}

	peerRelay := stun.XorPeerAddress{stun.XorAddress{IP: peerRelayAddr.IP, Port: peerRelayPort}}
	return stun.BuildAndSend(s.connection, peerRelayAddr, stun.ClassIndication, stun.MethodData, buildTransactionId(), &peerRelay, &dataAttr)
}

func (s *Server) handleChannelBindRequest(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
	errorSend := func(err error, attrs ...stun.Attribute) error {
		if sendErr := stun.BuildAndSend(s.connection, srcAddr, stun.ClassErrorResponse, stun.MethodChannelBind, m.TransactionID, attrs...); sendErr != nil {
			err = errors.Errorf(strings.Join([]string{sendErr.Error(), err.Error()}, "\n"))
		}
		return err
	}

	messageIntegrity, _, err := authenticateRequest(func(class stun.MessageClass, method stun.Method, transactionID []byte, attrs ...stun.Attribute) error {
		return stun.BuildAndSend(s.connection, srcAddr, class, method, transactionID, attrs...)
	}, m, stun.MethodChannelBind)
	if err != nil {
		return err
	}

	channel := stun.ChannelNumber{}
	peerAddr := stun.XorPeerAddress{}
	if cn, ok := m.GetOneAttribute(stun.AttrChannelNumber); ok {
		if err := channel.Unpack(m, cn); err != nil {
			return errorSend(err, &stun.Err400BadRequest)
		}
	} else {
		return errorSend(errors.Errorf("ChannelBind missing channel attribute"), &stun.Err400BadRequest)
	}
	if xpa, ok := m.GetOneAttribute(stun.AttrXORPeerAddress); ok {
		if err := peerAddr.Unpack(m, xpa); err != nil {
			return errorSend(err, &stun.Err400BadRequest)
		}
	} else {
		return errorSend(errors.Errorf("ChannelBind missing XORPeerAddress attribute"), &stun.Err400BadRequest)
	}

	if err := relayServer.AddChannelBind(peerAddr.XorAddress.Port, channel.ChannelNumber, srcAddr); err != nil {
		return errorSend(err, &stun.Err400BadRequest)
	}

	return stun.BuildAndSend(s.connection, srcAddr, stun.ClassSuccessResponse, stun.MethodChannelBind, m.TransactionID, messageIntegrity)
}

func (s *Server) handleChannelData(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, c *stun.ChannelData) error {
	clientAddr, ok := relayServer.GetChannelBind(srcAddr.Port, c.ChannelNumber)
	if !ok {
		return errors.Errorf("No channel bind found for %x", c.ChannelNumber)
	}

	l, err := s.connection.WriteTo(c.Data, nil, clientAddr.Addr())
	if err != nil {
		return errors.Wrap(err, "failed writing to socket")
	}

	if l != len(c.Data) {
		return errors.Errorf("packet write smaller than packet %d != %d (expected)", l, len(c.Data))
	}

	return nil
}

func addTurnHandlers(s *Server) {
	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodAllocate}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleAllocateRequest(srcAddr, dstAddr, m)
	}
	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodRefresh}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleRefreshRequest(srcAddr, dstAddr, m)
	}
	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodCreatePermission}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleCreatePermissionRequest(srcAddr, dstAddr, m)
	}
	s.handlers[HandlerKey{stun.ClassIndication, stun.MethodSend}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleSendIndication(srcAddr, dstAddr, m)
	}
	s.handlers[HandlerKey{stun.ClassRequest, stun.MethodChannelBind}] = func(srcAddr *stun.TransportAddr, dstAddr *stun.TransportAddr, m *stun.Message) error {
		return s.handleChannelBindRequest(srcAddr, dstAddr, m)
	}
}
