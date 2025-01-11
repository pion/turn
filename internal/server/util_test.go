// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateRequest(t *testing.T) {
	const (
		testUsername = "test-user"
		testRealm    = "test-realm"
	)
	testMsgIntegrity := stun.NewLongTermIntegrity(testUsername, testRealm, "pass")

	var conn net.PacketConn
	var nonce string
	var r *Request

	type options struct {
		noAuthHandler bool
	}

	setUp := func(t *testing.T, opts options) func() {
		var err error
		conn, err = net.ListenPacket("udp", "127.0.0.1:0")
		require.NoError(t, err)
		srcAddr := conn.LocalAddr()

		nonceHash, err := NewNonceHash()
		require.NoError(t, err)
		nonce, err = nonceHash.Generate()
		require.NoError(t, err)

		r = &Request{
			Conn:    conn,
			SrcAddr: srcAddr,
			AuthHandler: func(username, realm string, _ net.Addr) (key []byte, ok bool) {
				return testMsgIntegrity, username == testUsername && realm == testRealm
			},
			NonceHash: nonceHash,
		}
		if opts.noAuthHandler {
			r.AuthHandler = nil
		}

		return func() {
			err = conn.Close()
			if err != nil {
				t.Errorf("failed to close connection: %v", err)
			}
		}
	}

	checkSTUNAllocateErrorResponse := func(t *testing.T) stun.ErrorCode {
		// Set read deadline to avoid blocking for a long time
		err := conn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		require.NoError(t, err)

		// Check the error response
		buf := make([]byte, 1024)
		n, _, err := conn.ReadFrom(buf)
		require.NoError(t, err)

		resp := &stun.Message{}
		err = resp.UnmarshalBinary(buf[:n])
		require.NoError(t, err)
		require.Equal(t, stun.NewType(stun.MethodAllocate, stun.ClassErrorResponse), resp.Type)
		var attrErrorCode stun.ErrorCodeAttribute
		err = attrErrorCode.GetFrom(resp)
		require.NoError(t, err)
		return attrErrorCode.Code
	}

	checkNoSTUNResponse := func(t *testing.T) {
		// Set read deadline to avoid blocking for a long time
		err := conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		require.NoError(t, err)

		// Check the error response
		buf := make([]byte, 1024)
		_, _, err = conn.ReadFrom(buf)
		require.ErrorIs(t, err, os.ErrDeadlineExceeded)
	}

	t.Run("auth success", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			testMsgIntegrity,
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.NoError(t, err)
		require.True(t, authResult.hasAuth)
		require.Equal(t, testRealm, authResult.realm, "Realm value should be present in the result")
		require.Equal(t, testUsername, authResult.username, "Username value should be present in the result")

		checkNoSTUNResponse(t)
	})

	t.Run("no message integrity", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		// Message integrity attribute is missing
		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.NoError(t, err)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeUnauthorized, errorCode)
	})

	t.Run("no auth handler", func(t *testing.T) {
		tearDown := setUp(t, options{noAuthHandler: true})
		defer tearDown()

		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			testMsgIntegrity,
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.NoError(t, err)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})

	t.Run("no nonce", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		// Nonce attribute is missing
		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			testMsgIntegrity,
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.ErrorIs(t, err, stun.ErrAttributeNotFound)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})

	t.Run("invalid nonce", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			testMsgIntegrity,
			stun.NewNonce("bad nonce"), // <- bad nonce
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.NoError(t, err)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeStaleNonce, errorCode)
	})

	t.Run("no realm", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		// Realm attribute is missing
		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			testMsgIntegrity,
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.ErrorIs(t, err, stun.ErrAttributeNotFound)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})

	t.Run("no username", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		// Username attribute is missing
		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewRealm(testRealm),
			testMsgIntegrity,
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.ErrorIs(t, err, stun.ErrAttributeNotFound)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})

	t.Run("unknown username", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername("bad user"), // <- user name that does not exist
			stun.NewRealm(testRealm),
			testMsgIntegrity,
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.ErrorContains(t, err, "no such user")
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})

	t.Run("invalid message integrity", func(t *testing.T) {
		tearDown := setUp(t, options{})
		defer tearDown()

		m, err := stun.Build(
			stun.TransactionID,
			stun.NewType(stun.MethodAllocate, stun.ClassRequest),
			proto.RequestedTransport{Protocol: proto.ProtoUDP},
			stun.NewUsername(testUsername),
			stun.NewRealm(testRealm),
			stun.NewLongTermIntegrity(testUsername, testRealm, "bad"), // <- bad message integrity
			stun.NewNonce(nonce),
		)
		require.NoError(t, err)

		authResult, err := authenticateRequest(*r, m, stun.MethodAllocate)
		require.ErrorIs(t, err, stun.ErrIntegrityMismatch)
		require.False(t, authResult.hasAuth)

		// Check the error response
		errorCode := checkSTUNAllocateErrorResponse(t)
		require.Equal(t, stun.CodeBadRequest, errorCode)
	})
}
