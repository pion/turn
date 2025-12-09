// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildMsg(
	transactionID [stun.TransactionIDSize]byte,
	msgType stun.MessageType,
	additional ...stun.Setter,
) []stun.Setter {
	return append([]stun.Setter{&stun.Message{TransactionID: transactionID}, msgType}, additional...)
}

func createListeningTestClient(t *testing.T, loggerFactory logging.LoggerFactory) (*Client, net.PacketConn, bool) {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	c, err := NewClient(&ClientConfig{
		Conn:          conn,
		Software:      "TEST SOFTWARE",
		LoggerFactory: loggerFactory,
	})
	assert.NoError(t, err)
	assert.NoError(t, c.Listen())

	return c, conn, true
}

func createListeningTestClientWithSTUNServ(t *testing.T, loggerFactory logging.LoggerFactory) ( // nolint:lll
	*Client, net.PacketConn,
	bool,
) {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	addr := "stun1.l.google.com:19302"

	c, err := NewClient(&ClientConfig{
		STUNServerAddr: addr,
		Conn:           conn,
		LoggerFactory:  loggerFactory,
	})
	assert.NoError(t, err)
	assert.NoError(t, c.Listen())

	return c, conn, true
}

func TestClientWithSTUN(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	log := loggerFactory.NewLogger("test")

	t.Run("SendBindingRequest", func(t *testing.T) {
		client, pc, ok := createListeningTestClientWithSTUNServ(t, loggerFactory)
		if !ok {
			return
		}
		defer client.Close()

		resp, err := client.SendBindingRequest()
		assert.NoError(t, err, "should succeed")
		log.Debugf("mapped-addr: %s", resp)
		assert.Equal(t, 0, client.trMap.Size(), "should be no transaction left")
		assert.NoError(t, pc.Close())
	})

	t.Run("SendBindingRequestTo Parallel", func(t *testing.T) {
		client, pc, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer client.Close()

		// Simple channel fo go routine start signaling
		started := make(chan struct{})
		finished := make(chan struct{})
		var err1 error

		to, err := net.ResolveUDPAddr("udp4", "stun1.l.google.com:19302")
		assert.NoError(t, err)

		// stun1.l.google.com:19302, more at https://gist.github.com/zziuni/3741933#file-stuns-L5
		go func() {
			close(started)
			_, err1 = client.SendBindingRequestTo(to)
			close(finished)
		}()

		// Block until go routine is started to make two almost parallel requests
		<-started

		_, err = client.SendBindingRequestTo(to)
		assert.NoError(t, err)

		<-finished
		assert.NoErrorf(t, err1, "should succeed: %v", err)
		assert.NoError(t, pc.Close())
	})

	t.Run("NewClient should fail if Conn is nil", func(t *testing.T) {
		_, err := NewClient(&ClientConfig{
			LoggerFactory: loggerFactory,
		})
		assert.Error(t, err, "should fail")
	})

	t.Run("SendBindingRequestTo timeout", func(t *testing.T) {
		c, pc, ok := createListeningTestClient(t, loggerFactory)
		if !ok {
			return
		}
		defer c.Close()

		to, err := net.ResolveUDPAddr("udp4", "127.0.0.1:9")
		assert.NoError(t, err)

		c.rto = 10 * time.Millisecond // Force short timeout

		_, err = c.SendBindingRequestTo(to)
		assert.NotNil(t, err)
		assert.NoError(t, pc.Close())
	})
}

// Create an allocation, and then delete all nonces
// The subsequent Write on the allocation will cause a CreatePermission
// which will be forced to handle a stale nonce response.
func TestClientNonceExpiration(t *testing.T) {
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478") // nolint: noctx
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, realm string, _ net.Addr) (key []byte, ok bool) {
			return GenerateAuthKey(username, realm, "pass"), true
		},
		PacketConnConfigs: []PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "0.0.0.0",
				},
			},
		},
		Realm: "pion.ly",
	})
	assert.NoError(t, err)

	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	// nolint: goconst
	addr := "127.0.0.1:3478"

	client, err := NewClient(&ClientConfig{
		Conn:           conn,
		STUNServerAddr: addr,
		TURNServerAddr: addr,
		Username:       "foo",
		Password:       "pass",
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	allocation, err := client.Allocate()
	assert.NoError(t, err)

	_, err = allocation.WriteTo([]byte{0x00}, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080})
	assert.NoError(t, err)

	// Shutdown
	assert.NoError(t, allocation.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, server.Close())
}

// Create a TCP-based allocation and verify allocation can be created.
func TestTCPClient(t *testing.T) {
	// Setup server
	tcpListener, err := net.Listen("tcp4", "0.0.0.0:13478") //nolint: gosec,noctx
	require.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, realm string, _ net.Addr) (key []byte, ok bool) {
			return GenerateAuthKey(username, realm, "pass"), true
		},
		ListenerConfigs: []ListenerConfig{
			{
				Listener: tcpListener,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "0.0.0.0",
				},
			},
		},
		Realm: "pion.ly",
	})
	require.NoError(t, err)

	// Setup clients
	conn, err := net.Dial("tcp", "127.0.0.1:13478") // nolint: noctx
	require.NoError(t, err)

	serverAddr := "127.0.0.1:13478"

	client, err := NewClient(&ClientConfig{
		Conn:           NewSTUNConn(conn),
		STUNServerAddr: serverAddr,
		TURNServerAddr: serverAddr,
		Username:       "foo",
		Password:       "pass",
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())

	require.Equal(t, serverAddr, client.STUNServerAddr().String())

	allocation, err := client.AllocateTCP()
	require.NoError(t, err)

	peerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	require.NoError(t, err)
	require.NoError(t, client.CreatePermission(peerAddr))

	var cid proto.ConnectionID = 5
	transactionID := [stun.TransactionIDSize]byte{1, 2, 3}
	attrs := buildMsg(
		transactionID,
		stun.NewType(stun.MethodConnectionAttempt, stun.ClassIndication),
		cid,
		proto.PeerAddress{IP: peerAddr.IP, Port: peerAddr.Port},
	)

	msg, err := stun.Build(attrs...)
	require.NoError(t, err)

	require.NoError(t, client.handleSTUNMessage(msg.Raw, peerAddr))

	// Shutdown
	require.NoError(t, allocation.Close())
	require.NoError(t, conn.Close())
	require.NoError(t, server.Close())
}

func TestTCPClientWithoutAddress(t *testing.T) {
	// Setup server
	tcpListener, err := net.Listen("tcp4", "0.0.0.0:13478") //nolint: gosec,noctx
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(username, realm string, _ net.Addr) (key []byte, ok bool) {
			// Sleep needed for sending retransmission.
			if runtime.GOOS == "windows" {
				time.Sleep(20 * time.Millisecond)
			} else {
				time.Sleep(time.Millisecond)
			}

			return GenerateAuthKey(username, realm, "pass"), true
		},
		ListenerConfigs: []ListenerConfig{
			{
				Listener: tcpListener,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "0.0.0.0",
				},
			},
		},
		Realm: "pion.ly",
	})
	assert.NoError(t, err)

	// Test tcp client without turn server address with small RTO.
	conn, err := net.Dial("tcp", "127.0.0.1:13478") // nolint: noctx
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:     NewSTUNConn(conn),
		Username: "foo",
		Password: "pass",
		RTO:      time.Nanosecond,
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())
	defer client.Close()

	_, err = client.Allocate()
	// Due to small RTO all retrasmissions fail.
	assert.ErrorIs(t, err, errAllRetransmissionsFailed)
	assert.NoError(t, conn.Close())

	// Test tcp client without turn server with successful allocation.
	conn, err = net.Dial("tcp", "127.0.0.1:13478") // nolint: noctx

	assert.NoError(t, err)

	client, err = NewClient(&ClientConfig{
		Conn:     NewSTUNConn(conn),
		Username: "foo",
		Password: "pass",
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	relayConn, err := client.Allocate()
	assert.NoError(t, err)

	// Shutdown
	assert.NoError(t, relayConn.Close())
	assert.NoError(t, conn.Close())
	assert.NoError(t, server.Close())
}

func TestClientTURNResolving(t *testing.T) {
	listener, err := net.Listen("tcp4", "0.0.0.0:13478") //nolint: gosec
	assert.NoError(t, err)

	conn, err := net.Dial("tcp", "127.0.0.1:13478")
	assert.NoError(t, err)

	_, err = NewClient(&ClientConfig{
		TURNServerAddr: "unresolvable.turn.server.address:13478",
		Conn:           NewSTUNConn(conn),
	})
	assert.Error(t, err)

	_, err = NewClient(&ClientConfig{
		TURNServerAddr:          "unresolvable.turn.server.address:13478",
		Conn:                    NewSTUNConn(conn),
		IgnoreTURNResolveErrors: true,
	})
	assert.NoError(t, err)

	assert.NoError(t, conn.Close())
	assert.NoError(t, listener.Close())
}
