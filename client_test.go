// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package turn

import (
	"context"
	"errors"
	"io"
	"net"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5/vnet"
	"github.com/pion/turn/v5/internal/proto"
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
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	client, err := NewClient(&ClientConfig{
		Conn:           conn,
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
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
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	require.NoError(t, client.handleSTUNMessage(msg.Raw, conn.RemoteAddr()))

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
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			// Sleep needed for sending retransmission.
			if runtime.GOOS == "windows" {
				time.Sleep(20 * time.Millisecond)
			} else {
				time.Sleep(time.Millisecond)
			}

			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

func TestClientReadTimout(t *testing.T) {
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478") // nolint: noctx
	assert.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	stunClientConn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           stunClientConn,
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
		Username:       "foo",
		Password:       "pass",
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	allocation, err := client.Allocate()
	assert.NoError(t, err)

	assert.NoError(t, allocation.SetReadDeadline(time.Now().Add(time.Nanosecond)))
	_, _, err = allocation.ReadFrom(nil)
	assert.Contains(t, err.Error(), "i/o timeout")

	// Shutdown
	assert.NoError(t, allocation.Close())
	assert.NoError(t, stunClientConn.Close())
	assert.NoError(t, server.Close())

	_, _, err = allocation.ReadFrom(nil)
	assert.Contains(t, err.Error(), "use of closed network connection")
}

func TestTCPClientDial(t *testing.T) {
	tcpListener, err := net.Listen("tcp4", "0.0.0.0:3478") //nolint: gosec,noctx
	require.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	clientConn, err := net.Dial("tcp", testAddr) // nolint: noctx
	require.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           NewSTUNConn(clientConn),
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
		Username:       "foo",
		Password:       "pass",
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())

	allocation, err := client.AllocateTCP()
	assert.NoError(t, err)

	remotePeerConn, err := net.Listen("tcp4", "127.0.0.1:0") // nolint: noctx
	assert.NoError(t, err)

	hasReadCtx, hasReadCancel := context.WithCancel(context.Background())
	hasClosedCtx, hasClosedCancel := context.WithCancel(context.Background())

	expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	go func() {
		inboundTCPConn, inboundErr := remotePeerConn.Accept()
		assert.NoError(t, inboundErr)

		remoteAddr := inboundTCPConn.RemoteAddr()
		assert.Equal(t, allocation.Addr().String(), remoteAddr.String(), "peer conn: remote address = relay address")

		_, inboundErr = inboundTCPConn.Write(expectedMsg)
		assert.NoError(t, inboundErr)

		inboundBuffer := make([]byte, len(expectedMsg))
		_, inboundErr = inboundTCPConn.Read(inboundBuffer)
		assert.NoError(t, inboundErr)
		assert.Equal(t, expectedMsg, inboundBuffer)
		hasReadCancel()

		_, inboundErr = inboundTCPConn.Read(inboundBuffer)
		assert.ErrorIs(t, inboundErr, io.EOF)
		hasClosedCancel()
	}()

	remotePeerAddr, ok := remotePeerConn.Addr().(*net.TCPAddr)
	assert.True(t, ok)

	connectionBindConn, err := allocation.Dial("tcp4", remotePeerAddr.String())
	assert.NoError(t, err)

	localAddr := connectionBindConn.LocalAddr()
	assert.Equal(t, allocation.Addr().String(), localAddr.String(), "client conn: local address = relay address")

	channelBindConnBuffer := make([]byte, len(expectedMsg))
	_, err = connectionBindConn.Read(channelBindConnBuffer)
	assert.NoError(t, err)
	assert.Equal(t, expectedMsg, channelBindConnBuffer)

	_, err = connectionBindConn.Write(expectedMsg)
	assert.NoError(t, err)
	<-hasReadCtx.Done()

	// Closing the ConnectionBind must close the TCP Socket
	assert.NoError(t, connectionBindConn.Close())

	// Shutdown
	<-hasClosedCtx.Done()
	assert.NoError(t, allocation.Close())
	assert.NoError(t, clientConn.Close())
	assert.NoError(t, server.Close())
	assert.NoError(t, remotePeerConn.Close())
}

func TestTCPClientAccept(t *testing.T) {
	tcpListener, err := net.Listen("tcp4", "0.0.0.0:3478") //nolint: gosec,noctx
	require.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	clientConn, err := net.Dial("tcp", testAddr) // nolint: noctx
	require.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           NewSTUNConn(clientConn),
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
		Username:       "foo",
		Password:       "pass",
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())

	allocation, err := client.AllocateTCP()
	assert.NoError(t, err)

	// Install a permission for the loopback peer before it connects (RFC 6062 §5).
	require.NoError(t, client.CreatePermission(&net.TCPAddr{IP: net.ParseIP("127.0.0.1")}))

	expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	peerConn, err := net.Dial("tcp4", allocation.Addr().String()) // nolint: noctx
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_, wErr := peerConn.Write(expectedMsg)
		assert.NoError(t, wErr)

		peerBuf := make([]byte, len(expectedMsg))
		_, rErr := peerConn.Read(peerBuf)
		assert.NoError(t, rErr)
		assert.Equal(t, expectedMsg, peerBuf)

		cancel()
	}()

	allocationConn, err := allocation.Accept()
	assert.NoError(t, err)

	allocationConnBuffer := make([]byte, len(expectedMsg))
	_, err = allocationConn.Read(allocationConnBuffer)
	assert.NoError(t, err)
	assert.Equal(t, expectedMsg, allocationConnBuffer)

	_, err = allocationConn.Write(expectedMsg)
	assert.NoError(t, err)
	<-ctx.Done()

	// Shutdown
	assert.NoError(t, peerConn.Close())
	assert.NoError(t, allocation.Close())
	assert.NoError(t, clientConn.Close())
	assert.NoError(t, server.Close())
}

func TestTCPClientMultipleConns(t *testing.T) {
	tcpListener, err := net.Listen("tcp4", "0.0.0.0:3478") //nolint: gosec,noctx
	require.NoError(t, err)

	server, err := NewServer(ServerConfig{
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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

	clientConn, err := net.Dial("tcp", testAddr) // nolint: noctx
	require.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           NewSTUNConn(clientConn),
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
		Username:       "foo",
		Password:       "pass",
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())

	allocation, err := client.AllocateTCP()
	assert.NoError(t, err)

	// Install a permission for the loopback peer before it connects (RFC 6062 §5).
	require.NoError(t, client.CreatePermission(&net.TCPAddr{IP: net.ParseIP("127.0.0.1")}))

	var wg sync.WaitGroup
	runPeerDialer := func(i int) net.Conn {
		relayAddr, ok := allocation.Addr().(*net.TCPAddr)
		assert.True(t, ok)

		expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF, byte(i)} //nolint:gosec // no overflow, test sample.
		peerConn, peerErr := net.DialTCP("tcp4", nil, relayAddr)
		assert.NoError(t, peerErr)

		peerBuffer := make([]byte, len(expectedMsg))
		_, peerErr = peerConn.Read(peerBuffer)
		assert.NoError(t, peerErr)
		assert.Equal(t, expectedMsg, peerBuffer)

		_, peerErr = peerConn.Write(expectedMsg)
		assert.NoError(t, peerErr)

		return peerConn
	}

	runPeerAcceptor := func(ctx context.Context, i int) (net.Listener, net.Addr) {
		remotePeerListener, err := net.Listen("tcp4", "127.0.0.1:0") // nolint: noctx
		assert.NoError(t, err)

		expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF, byte(i)} //nolint:gosec // no overflow, test sample.
		wg.Add(1)
		go func() {
			defer wg.Done()

			peerConn, peerErr := remotePeerListener.Accept()
			assert.NoError(t, peerErr)

			peerBuffer := make([]byte, len(expectedMsg))
			_, peerErr = peerConn.Read(peerBuffer)
			assert.NoError(t, peerErr)
			assert.Equal(t, expectedMsg, peerBuffer)

			_, peerErr = peerConn.Write(expectedMsg)
			assert.NoError(t, peerErr)

			<-ctx.Done()

			assert.NoError(t, peerConn.Close())
		}()

		return remotePeerListener, remotePeerListener.Addr()
	}

	runClientDialer := func(remotePeerAddr net.Addr, i int) net.Conn {
		dialerConn, err := allocation.Dial("tcp4", remotePeerAddr.String())
		assert.NoError(t, err)

		expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF, byte(i)} //nolint:gosec // no overflow, test sample.
		_, err = dialerConn.Write(expectedMsg)
		assert.NoError(t, err)

		clientBuffer := make([]byte, len(expectedMsg))
		_, err = dialerConn.Read(clientBuffer)
		assert.NoError(t, err)
		assert.Equal(t, expectedMsg, clientBuffer)

		return dialerConn
	}

	runClientAcceptor := func(ctx context.Context, i int) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			acceptorConn, err := allocation.Accept()
			assert.NoError(t, err)

			expectedMsg := []byte{0xDE, 0xAD, 0xBE, 0xEF, byte(i)} //nolint:gosec // no overflow, test sample.
			_, err = acceptorConn.Write(expectedMsg)
			assert.NoError(t, err)

			clientBuffer := make([]byte, len(expectedMsg))
			_, err = acceptorConn.Read(clientBuffer)
			assert.NoError(t, err)
			assert.Equal(t, expectedMsg, clientBuffer)

			<-ctx.Done()

			assert.NoError(t, acceptorConn.Close())
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	clientConns := []net.Conn{}
	peerConns := []net.Conn{}
	peerListeners := []net.Listener{}
	for i := range 3 {
		// client -> server -> peer
		peerListener, peerAddr := runPeerAcceptor(ctx, i)
		time.Sleep(time.Second)
		peerListeners = append(peerListeners, peerListener)
		dialerConn := runClientDialer(peerAddr, i)
		clientConns = append(clientConns, dialerConn)

		// peer -> server -> client
		runClientAcceptor(ctx, i)
		peerConn := runPeerDialer(i)
		peerConns = append(peerConns, peerConn)
	}

	cancel()
	wg.Wait()

	// Shutdown
	for i := range 3 {
		assert.NoError(t, peerListeners[i].Close())
		assert.NoError(t, peerConns[i].Close())
		assert.NoError(t, clientConns[i].Close())
	}
	assert.NoError(t, allocation.Close())
	assert.NoError(t, server.Close())
	assert.NoError(t, clientConn.Close())
}

func TestInferAddressFamilyFromConn(t *testing.T) {
	t.Run("IPv4 UDP connection", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		family, err := inferAddressFamilyFromConn(conn)
		assert.NoError(t, err)
		assert.Equal(t, proto.RequestedFamilyIPv4, family)
	})

	t.Run("IPv6 UDP connection", func(t *testing.T) {
		conn, err := net.ListenPacket("udp6", "[::]:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		family, err := inferAddressFamilyFromConn(conn)
		assert.NoError(t, err)
		assert.Equal(t, proto.RequestedFamilyIPv6, family)
	})
}

func TestGetRequestedAddressFamily(t *testing.T) {
	log := logging.NewDefaultLoggerFactory().NewLogger("test")

	t.Run("Explicit IPv4 in config", func(t *testing.T) {
		conn, err := net.ListenPacket("udp6", "[::]:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		config := &ClientConfig{
			Conn:                   conn,
			RequestedAddressFamily: proto.RequestedFamilyIPv4,
		}

		// Should use explicit config even though conn is IPv6
		family := getRequestedAddressFamily(log, config)
		assert.Equal(t, proto.RequestedFamilyIPv4, family)
	})

	t.Run("Explicit IPv6 in config", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		config := &ClientConfig{
			Conn:                   conn,
			RequestedAddressFamily: proto.RequestedFamilyIPv6,
		}

		// Should use explicit config even though conn is IPv4
		family := getRequestedAddressFamily(log, config)
		assert.Equal(t, proto.RequestedFamilyIPv6, family)
	})

	t.Run("Infer IPv4 from connection", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		config := &ClientConfig{
			Conn: conn,
			// RequestedAddressFamily not set (zero value)
		}

		// Should infer IPv4 from connection
		family := getRequestedAddressFamily(log, config)
		assert.Equal(t, proto.RequestedFamilyIPv4, family)
	})

	t.Run("Infer IPv6 from connection", func(t *testing.T) {
		conn, err := net.ListenPacket("udp6", "[::]:0") //nolint:noctx
		assert.NoError(t, err)
		defer conn.Close() //nolint:errcheck

		config := &ClientConfig{
			Conn: conn,
			// RequestedAddressFamily not set (zero value)
		}

		// Should infer IPv6 from connection
		family := getRequestedAddressFamily(log, config)
		assert.Equal(t, proto.RequestedFamilyIPv6, family)
	})
}

func TestAppendRequestedAddressFamilyOrReservation(t *testing.T) {
	t.Run("IPv4 omits attribute", func(t *testing.T) {
		setters := appendRequestedAddressFamilyOrReservation(
			[]stun.Setter{stun.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassRequest)},
			proto.RequestedFamilyIPv4,
			nil,
		)

		msg, err := stun.Build(setters...)
		require.NoError(t, err)

		assert.False(t, msg.Contains(stun.AttrRequestedAddressFamily))
	})

	t.Run("IPv6 includes attribute", func(t *testing.T) {
		setters := appendRequestedAddressFamilyOrReservation(
			[]stun.Setter{stun.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassRequest)},
			proto.RequestedFamilyIPv6,
			nil,
		)

		msg, err := stun.Build(setters...)
		require.NoError(t, err)

		var raf proto.RequestedAddressFamily
		require.NoError(t, raf.GetFrom(msg))
		assert.Equal(t, proto.RequestedFamilyIPv6, raf)
	})

	t.Run("Reservation token takes precedence", func(t *testing.T) {
		token := proto.ReservationToken{1, 2, 3, 4, 5, 6, 7, 8}
		setters := appendRequestedAddressFamilyOrReservation(
			[]stun.Setter{stun.TransactionID, stun.NewType(stun.MethodAllocate, stun.ClassRequest)},
			proto.RequestedFamilyIPv6,
			token,
		)

		msg, err := stun.Build(setters...)
		require.NoError(t, err)

		var parsedToken proto.ReservationToken
		require.NoError(t, parsedToken.GetFrom(msg))
		assert.Equal(t, token, parsedToken)
		assert.False(t, msg.Contains(stun.AttrRequestedAddressFamily))
	})
}

type channelBindFilterConn struct {
	net.PacketConn

	doFilter bool
}

func (c *channelBindFilterConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	for {
		n, addr, err = c.PacketConn.ReadFrom(p)

		if c.doFilter {
			stunMsg := &stun.Message{Raw: p[:n]}
			if err := stunMsg.Decode(); err == nil && stunMsg.Type.Method == stun.MethodChannelBind {
				continue
			}
		}

		return
	}
}

func TestClientE2E(t *testing.T) {
	doTest := func(disableChannelBind bool) {
		udpListener, err := net.ListenPacket("udp4", "0.0.0.0:3478") // nolint: noctx
		assert.NoError(t, err)

		server, err := NewServer(ServerConfig{
			AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
				return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
			},
			PacketConnConfigs: []PacketConnConfig{
				{
					PacketConn: &channelBindFilterConn{udpListener, disableChannelBind},
					RelayAddressGenerator: &RelayAddressGeneratorStatic{
						RelayAddress: net.ParseIP("127.0.0.1"),
						Address:      "0.0.0.0",
					},
				},
			},
			Realm:              "pion.ly",
			AllocationLifetime: time.Second,
			PermissionTimeout:  time.Millisecond * 100,
			ChannelBindTimeout: time.Millisecond * 100,
		})
		assert.NoError(t, err)

		stunClientConn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
		assert.NoError(t, err)

		client, err := NewClient(&ClientConfig{
			Conn:                      stunClientConn,
			STUNServerAddr:            testAddr,
			TURNServerAddr:            testAddr,
			Username:                  "foo",
			Password:                  "pass",
			PermissionRefreshInterval: time.Millisecond * 50,
			bindingRefreshInterval:    time.Millisecond * 50,
			bindingCheckInterval:      time.Millisecond * 50,
		})
		assert.NoError(t, err)
		assert.NoError(t, client.Listen())

		allocation, err := client.Allocate()
		assert.NoError(t, err)

		remotePeerConn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
		assert.NoError(t, err)

		remotePeerAddr, ok := remotePeerConn.LocalAddr().(*net.UDPAddr)
		assert.True(t, ok)

		allocationAddr, ok := allocation.LocalAddr().(*net.UDPAddr)
		assert.True(t, ok)

		sendPackets := func(src, dst net.PacketConn, port int) {
			const expectedPktCount = 25
			expectedPacket := []byte{0xDE, 0xAD, 0xBE, 0xEF}

			pktCount := atomic.Uint32{}
			go func() {
				buff := make([]byte, len(expectedPacket))
				for pktCount.Load() < expectedPktCount {
					i, _, readErr := dst.ReadFrom(buff)
					assert.NoError(t, readErr)

					assert.Equal(t, expectedPacket, buff[:i])
					pktCount.Add(1)
				}
			}()
			for pktCount.Load() < expectedPktCount {
				_, err = src.WriteTo(expectedPacket, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
				assert.NoError(t, err)

				time.Sleep(time.Millisecond * 25)
			}
		}

		sendPackets(allocation, remotePeerConn, remotePeerAddr.Port)
		sendPackets(remotePeerConn, allocation, allocationAddr.Port)

		assert.NotNil(t, client.TURNServerAddr())
		assert.NotNil(t, client.Username())
		assert.NotNil(t, client.Realm())

		// Shutdown
		assert.NoError(t, remotePeerConn.Close())
		assert.NoError(t, allocation.Close())
		assert.NoError(t, stunClientConn.Close())
		assert.NoError(t, server.Close())
	}

	doTest(true)
	doTest(false)
}

// Create an allocation and use the context to cancel it.
func TestClientAllocateContext(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	client, err := NewClient(&ClientConfig{
		Conn:           conn,
		STUNServerAddr: testAddr,
		TURNServerAddr: testAddr,
		Username:       "foo",
		Password:       "pass",
	})
	assert.NoError(t, err)
	assert.NoError(t, client.Listen())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.AllocateWithContext(ctx)
	assert.Error(t, err)

	// Shutdown
	assert.NoError(t, conn.Close())
}

func TestClientRelaySource(t *testing.T) {
	udpConn, err := net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, udpConn.Close()) })
	stream, peer := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, stream.Close())
		require.NoError(t, peer.Close())
	})
	server := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478}
	attacker := &net.UDPAddr{IP: server.IP, Port: 3479}
	for _, conn := range []net.PacketConn{udpConn, NewSTUNConn(stream)} {
		turnClient, clientErr := NewClient(&ClientConfig{Conn: conn, TURNServerAddr: server.String()})
		require.NoError(t, clientErr)
		trusted := net.Addr(server)
		if conn != udpConn {
			trusted = stream.RemoteAddr()
			require.False(t, turnClient.isTURNServer(server), "use the connected endpoint, not the resolved address")
		}
		// An invalid indication must be dropped before parsing its untrusted payload.
		indication := stun.MustBuild(stun.TransactionID, stun.NewType(stun.MethodData, stun.ClassIndication))
		require.True(t, turnClient.isTURNServer(trusted))
		for _, packet := range []struct {
			data      []byte
			wantError bool
		}{{[]byte{0x40, 0, 0, 0}, false}, {indication.Raw, true}} {
			for _, from := range []net.Addr{attacker, nil} {
				handled, inboundErr := turnClient.HandleInbound(packet.data, from)
				require.True(t, handled)
				require.NoError(t, inboundErr)
			}
			handled, inboundErr := turnClient.HandleInbound(packet.data, trusted)
			require.True(t, handled)
			require.Equal(t, packet.wantError, inboundErr != nil, "trusted relay packets reach the handler")
		}
		turnClient.Close()
	}
}

var errTestCredentialLookup = errors.New("credential lookup failed")

type tryAlternateCtxKey struct{}

// startTestServer runs a TURN server that accepts the users foo:pass and bar:secret.
func startTestServer(t *testing.T, config ServerConfig) {
	t.Helper()

	config.Realm = "pion.ly"
	config.AuthHandler = func(ra *RequestAttributes) (string, []byte, bool) {
		password, ok := map[string]string{"foo": "pass", "bar": "secret"}[ra.Username]

		return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, password), ok
	}
	server, err := NewServer(config)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, server.Close()) })
}

// listenTestServer runs startTestServer on a loopback UDP port and returns its address.
func listenTestServer(t *testing.T) net.Addr {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	startTestServer(t, ServerConfig{PacketConnConfigs: []PacketConnConfig{{
		PacketConn:            conn,
		RelayAddressGenerator: &RelayAddressGeneratorStatic{RelayAddress: net.ParseIP("127.0.0.1"), Address: "0.0.0.0"},
	}}})

	return conn.LocalAddr()
}

// serveTryAlternate answers every request on conn as relay.communication.microsoft.com does:
// 300 (Try Alternate) with NONCE, REALM and attrs, even when the request is authenticated.
// It returns the number of distinct requests received.
func serveTryAlternate(t *testing.T, conn net.PacketConn, attrs ...stun.Setter) *atomic.Int32 {
	t.Helper()

	var requests atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		seen := map[[stun.TransactionIDSize]byte]struct{}{}
		buf := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			req := &stun.Message{Raw: append([]byte(nil), buf[:n]...)}
			if req.Decode() != nil {
				continue
			}
			if _, ok := seen[req.TransactionID]; !ok {
				seen[req.TransactionID] = struct{}{}
				requests.Add(1)
			}
			setters := append([]stun.Setter{
				&stun.Message{TransactionID: req.TransactionID},
				stun.NewType(req.Type.Method, stun.ClassErrorResponse),
				stun.CodeTryAlternate, stun.NewNonce("nonce"), stun.NewRealm("rtcmedia"),
			}, attrs...)
			res, err := stun.Build(append(setters, stun.Fingerprint)...)
			if assert.NoError(t, err) {
				_, _ = conn.WriteTo(res.Raw, from)
			}
		}
	}()
	t.Cleanup(func() {
		assert.NoError(t, conn.Close())
		<-done
	})

	return &requests
}

func alternateAttr(t *testing.T, addr net.Addr) *stun.AlternateServer {
	t.Helper()

	switch addr := addr.(type) {
	case *net.UDPAddr:
		return &stun.AlternateServer{IP: addr.IP, Port: addr.Port}
	case *net.TCPAddr:
		return &stun.AlternateServer{IP: addr.IP, Port: addr.Port}
	default:
		assert.Failf(t, "unexpected address type", "%T", addr)

		return nil
	}
}

// newUDPTestClient returns a listening client on a loopback UDP port.
func newUDPTestClient(t *testing.T, config ClientConfig) *Client {
	t.Helper()

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	config.Conn = conn
	client, err := NewClient(&config)
	require.NoError(t, err)
	require.NoError(t, client.Listen())
	t.Cleanup(func() {
		client.Close()
		assert.NoError(t, conn.Close())
	})

	return client
}

// newTCPTestClient returns a listening client on conn, configured like pion/ice does over TCP.
func newTCPTestClient(t *testing.T, conn net.Conn, handler TryAlternateHandler) *Client {
	t.Helper()

	client, err := NewClient(&ClientConfig{
		Conn:                NewSTUNConn(conn),
		Username:            "foo",
		Password:            "pass",
		TryAlternateHandler: handler,
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())
	t.Cleanup(func() {
		client.Close()
		assert.NoError(t, conn.Close())
	})

	return client
}

// tryAlternateConn stands in for TURN servers over a packet transport. It answers the n-th
// distinct request with an error response carrying respond(n).
type tryAlternateConn struct {
	client  *Client
	respond func(n int) []stun.Setter

	mu     sync.Mutex
	seen   map[[stun.TransactionIDSize]byte]int
	sentTo []string
}

func newStubClient(t *testing.T, server string, respond func(n int) []stun.Setter) (*Client, *tryAlternateConn) {
	t.Helper()

	stub := &tryAlternateConn{respond: respond, seen: map[[stun.TransactionIDSize]byte]int{}}
	client, err := NewClient(&ClientConfig{Conn: stub, TURNServerAddr: server, Username: "foo", Password: "pass"})
	require.NoError(t, err)
	stub.client = client
	t.Cleanup(client.Close)

	return client, stub
}

func (c *tryAlternateConn) WriteTo(packet []byte, addr net.Addr) (int, error) {
	req := &stun.Message{Raw: append([]byte(nil), packet...)}
	if err := req.Decode(); err != nil {
		return 0, err
	}
	c.mu.Lock()
	n, ok := c.seen[req.TransactionID]
	if !ok {
		c.sentTo = append(c.sentTo, addr.String())
		n = len(c.sentTo)
		c.seen[req.TransactionID] = n
	}
	c.mu.Unlock()

	setters := append([]stun.Setter{
		&stun.Message{TransactionID: req.TransactionID},
		stun.NewType(req.Type.Method, stun.ClassErrorResponse),
	}, c.respond(n)...)
	res, err := stun.Build(append(setters, stun.Fingerprint)...)
	if err != nil {
		return 0, err
	}
	go func() { _, _ = c.client.HandleInbound(res.Raw, addr) }()

	return len(packet), nil
}

// destinations returns the servers each distinct request was sent to.
func (c *tryAlternateConn) destinations() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return slices.Clone(c.sentTo)
}

func (c *tryAlternateConn) ReadFrom([]byte) (int, net.Addr, error) { return 0, nil, io.EOF }
func (c *tryAlternateConn) Close() error                           { return nil }
func (c *tryAlternateConn) LocalAddr() net.Addr                    { return &net.UDPAddr{IP: net.IPv4zero} }
func (c *tryAlternateConn) SetDeadline(time.Time) error            { return nil }
func (c *tryAlternateConn) SetReadDeadline(time.Time) error        { return nil }
func (c *tryAlternateConn) SetWriteDeadline(time.Time) error       { return nil }

// A 300 (Try Alternate) carrying NONCE and REALM is followed, not answered with credentials.
func TestClientAllocateFollowsTryAlternate(t *testing.T) {
	alternate := listenTestServer(t)
	redirectConn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	requests := serveTryAlternate(t, redirectConn, alternateAttr(t, alternate))

	client := newUDPTestClient(t, ClientConfig{
		TURNServerAddr: redirectConn.LocalAddr().String(),
		Username:       "foo",
		Password:       "pass",
	})
	relayConn, err := client.Allocate()
	require.NoError(t, err)
	assert.Equal(t, alternate.String(), client.TURNServerAddr().String())
	assert.Equal(t, int32(1), requests.Load(), "no authenticated request to the redirecting server")
	assert.NoError(t, relayConn.Close())
}

func TestClientTryAlternateHandler(t *testing.T) {
	setup := func(t *testing.T, password string, handler TryAlternateHandler) (*Client, net.Addr, net.Addr) {
		t.Helper()

		alternate := listenTestServer(t)
		redirectConn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
		require.NoError(t, err)
		serveTryAlternate(t, redirectConn, alternateAttr(t, alternate))
		client := newUDPTestClient(t, ClientConfig{
			TURNServerAddr:      redirectConn.LocalAddr().String(),
			Username:            "foo",
			Password:            password,
			TryAlternateHandler: handler,
		})

		return client, redirectConn.LocalAddr(), alternate
	}

	t.Run("new credentials", func(t *testing.T) {
		var got *TryAlternateError
		client, server, alternate := setup(t, "expired", func(ctx context.Context, redirect *TryAlternateError) (string, string, error) {
			assert.Equal(t, "allocate", ctx.Value(tryAlternateCtxKey{}))
			got = redirect

			return "bar", "secret", nil
		})

		ctx := context.WithValue(context.Background(), tryAlternateCtxKey{}, "allocate")
		relayConn, err := client.AllocateWithContext(ctx)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, server.String(), got.Server.String())
		assert.Equal(t, alternate.String(), got.Alternate.String())
		assert.Equal(t, "bar", client.Username().String())
		assert.NoError(t, relayConn.Close())
	})

	t.Run("error stops the redirect", func(t *testing.T) {
		client, server, _ := setup(t, "pass", func(context.Context, *TryAlternateError) (string, string, error) {
			return "", "", errTestCredentialLookup
		})

		_, err := client.Allocate()
		assert.ErrorIs(t, err, errTestCredentialLookup)
		assert.Equal(t, server.String(), client.TURNServerAddr().String())
	})

	t.Run("declined", func(t *testing.T) {
		client, server, alternate := setup(t, "pass", func(_ context.Context, redirect *TryAlternateError) (string, string, error) {
			return "", "", redirect
		})

		_, err := client.Allocate()
		var redirect *TryAlternateError
		require.ErrorAs(t, err, &redirect)
		assert.Equal(t, alternate.String(), redirect.Alternate.String())
		var turnErr *stun.TurnError
		require.ErrorAs(t, err, &turnErr)
		assert.Equal(t, stun.CodeTryAlternate, turnErr.ErrorCodeAttr.Code)
		assert.Equal(t, server.String(), client.TURNServerAddr().String())
	})
}

// Two servers redirecting to each other are each sent the request once.
func TestClientTryAlternateLoop(t *testing.T) {
	first, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	second, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	firstRequests := serveTryAlternate(t, first, alternateAttr(t, second.LocalAddr()))
	secondRequests := serveTryAlternate(t, second, alternateAttr(t, first.LocalAddr()))

	client := newUDPTestClient(t, ClientConfig{TURNServerAddr: first.LocalAddr().String(), Username: "foo", Password: "pass"})
	_, err = client.Allocate()
	assert.ErrorIs(t, err, errAlternateServerLoop)
	var redirect *TryAlternateError
	assert.ErrorAs(t, err, &redirect)
	assert.Equal(t, int32(1), firstRequests.Load())
	assert.Equal(t, int32(1), secondRequests.Load())
}

// An endless chain of distinct alternates is cut off after maxAlternateRedirects.
func TestClientTryAlternateLimit(t *testing.T) {
	client, stub := newStubClient(t, "127.0.0.1:3478", func(n int) []stun.Setter {
		return []stun.Setter{stun.CodeTryAlternate, &stun.AlternateServer{IP: net.IPv4(127, 0, 0, 1), Port: 3478 + n}}
	})

	_, err := client.Allocate()
	assert.ErrorIs(t, err, errTooManyAlternateRedirects)
	var redirect *TryAlternateError
	assert.ErrorAs(t, err, &redirect)
	assert.Len(t, stub.destinations(), maxAlternateRedirects+1)
}

// Invalid ALTERNATE-SERVER attributes and ones of the other address family are skipped.
func TestClientTryAlternateAttributes(t *testing.T) {
	alternate := func(ip string, port int) stun.Setter { return &stun.AlternateServer{IP: net.ParseIP(ip), Port: port} }
	raw := func(value ...byte) stun.Setter {
		return stun.RawAttribute{Type: stun.AttrAlternateServer, Value: value}
	}
	for _, test := range []struct {
		name   string
		server string
		attrs  []stun.Setter
		sentTo []string // Servers the request is sent to before Allocate fails.
	}{
		{"missing", "127.0.0.1:3478", nil, []string{"127.0.0.1:3478"}},
		{"truncated", "127.0.0.1:3478", []stun.Setter{raw(0, 1, 0x0d, 0x96, 127, 0)}, []string{"127.0.0.1:3478"}},
		{"unknown family", "127.0.0.1:3478", []stun.Setter{raw(0, 3, 0x0d, 0x96, 127, 0, 0, 2)}, []string{"127.0.0.1:3478"}},
		{"unspecified address", "127.0.0.1:3478", []stun.Setter{alternate("0.0.0.0", 3478)}, []string{"127.0.0.1:3478"}},
		{"zero port", "127.0.0.1:3478", []stun.Setter{alternate("127.0.0.2", 0)}, []string{"127.0.0.1:3478"}},
		{"other family only", "127.0.0.1:3478", []stun.Setter{alternate("2001:db8::1", 3478)}, []string{"127.0.0.1:3478"}},
		{
			"IPv4 server skips IPv6", "127.0.0.1:3478",
			[]stun.Setter{alternate("2001:db8::1", 3478), alternate("127.0.0.2", 3478)},
			[]string{"127.0.0.1:3478", "127.0.0.2:3478"},
		},
		{
			"IPv6 server skips IPv4", "[::1]:3478",
			[]stun.Setter{alternate("127.0.0.2", 3478), alternate("::2", 3478)},
			[]string{"[::1]:3478", "[::2]:3478"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, stub := newStubClient(t, test.server, func(int) []stun.Setter {
				return append([]stun.Setter{stun.CodeTryAlternate}, test.attrs...)
			})

			_, err := client.Allocate()
			var redirect *TryAlternateError
			require.ErrorAs(t, err, &redirect)
			assert.Equal(t, test.sentTo, stub.destinations())
			if len(test.sentTo) == 1 {
				assert.Nil(t, redirect.Alternate)
			} else {
				assert.ErrorIs(t, err, errAlternateServerLoop, "the alternate redirects to itself")
			}
		})
	}
}

// A TryAlternateError built by a caller, such as a handler test, has no TURN error to unwrap.
func TestTryAlternateErrorWithoutResponse(t *testing.T) {
	redirect := &TryAlternateError{Server: &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 3478}}
	assert.Equal(t, "turn: 1.2.3.4:3478 answered 300 (Try Alternate) without a valid ALTERNATE-SERVER", redirect.Error())
	assert.NoError(t, errors.Unwrap(redirect))

	redirect.Alternate = &net.UDPAddr{IP: net.IPv4(1, 2, 3, 5), Port: 3478}
	assert.Equal(t, "turn: 1.2.3.4:3478 answered 300 (Try Alternate) with alternate server 1.2.3.5:3478", redirect.Error())
}

// Only 401 and 438 are challenges; another error carrying NONCE and REALM ends the Allocate.
func TestClientAllocateErrorWithNonce(t *testing.T) {
	client, stub := newStubClient(t, "127.0.0.1:3478", func(int) []stun.Setter {
		return []stun.Setter{stun.CodeForbidden, stun.NewNonce("nonce"), stun.NewRealm("pion.ly")}
	})

	_, err := client.Allocate()
	var turnErr *stun.TurnError
	require.ErrorAs(t, err, &turnErr)
	assert.Equal(t, stun.CodeForbidden, turnErr.ErrorCodeAttr.Code)
	assert.Len(t, stub.destinations(), 1)
}

// Over TCP the redirect is returned with a TCP alternate, for the caller to connect to.
func TestTCPClientTryAlternate(t *testing.T) {
	alternateListener, err := net.Listen("tcp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	startTestServer(t, ServerConfig{ListenerConfigs: []ListenerConfig{{
		Listener:              alternateListener,
		RelayAddressGenerator: &RelayAddressGeneratorStatic{RelayAddress: net.ParseIP("127.0.0.1"), Address: "0.0.0.0"},
	}}})
	redirectListener, err := net.Listen("tcp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, redirectListener.Close()) })

	for _, test := range []struct {
		name     string
		allocate func(*Client) (io.Closer, error)
	}{
		{"Allocate", func(c *Client) (io.Closer, error) { return c.Allocate() }},
		{"AllocateTCP", func(c *Client) (io.Closer, error) { return c.AllocateTCP() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, err := net.Dial("tcp4", redirectListener.Addr().String()) // nolint: noctx
			require.NoError(t, err)
			accepted, err := redirectListener.Accept()
			require.NoError(t, err)
			serveTryAlternate(t, NewSTUNConn(accepted), alternateAttr(t, alternateListener.Addr()),
				stun.RawAttribute{Type: stun.AttrAlternateDomain, Value: []byte("turn.pion.ly")})
			client := newTCPTestClient(t, conn, func(context.Context, *TryAlternateError) (string, string, error) {
				assert.Fail(t, "redirects over TCP are left to the caller")

				return "", "", errTestCredentialLookup
			})

			_, err = test.allocate(client)
			var redirect *TryAlternateError
			require.ErrorAs(t, err, &redirect)
			assert.Equal(t, conn.RemoteAddr().String(), redirect.Server.String())
			assert.IsType(t, &net.TCPAddr{}, redirect.Alternate)
			assert.Equal(t, alternateListener.Addr().String(), redirect.Alternate.String())
			assert.Equal(t, "turn.pion.ly", redirect.Domain)

			alternateConn, err := net.Dial(redirect.Alternate.Network(), redirect.Alternate.String()) // nolint: noctx
			require.NoError(t, err)
			allocation, err := test.allocate(newTCPTestClient(t, alternateConn, nil))
			require.NoError(t, err)
			assert.NoError(t, allocation.Close())
		})
	}
}

// A redirect shaped like relay.communication.microsoft.com's, end to end on a virtual network:
// data relayed by the alternate reaches the client.
func TestClientTryAlternateVNet(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	wan, err := vnet.NewRouter(&vnet.RouterConfig{CIDR: "0.0.0.0/0", LoggerFactory: loggerFactory})
	require.NoError(t, err)
	newNet := func(ip string) *vnet.Net {
		vnetNet, netErr := vnet.NewNet(&vnet.NetConfig{StaticIPs: []string{ip}})
		require.NoError(t, netErr)
		require.NoError(t, wan.AddNet(vnetNet))

		return vnetNet
	}
	redirectNet, turnNet, clientNet, peerNet := newNet("1.2.3.4"), newNet("1.2.3.5"), newNet("1.2.3.6"), newNet("1.2.3.7")
	require.NoError(t, wan.AddHost("turn.pion.ly", "1.2.3.4"))
	require.NoError(t, wan.Start())
	t.Cleanup(func() { assert.NoError(t, wan.Stop()) })

	turnConn, err := turnNet.ListenPacket("udp4", "1.2.3.5:3478")
	require.NoError(t, err)
	startTestServer(t, ServerConfig{
		PacketConnConfigs: []PacketConnConfig{{
			PacketConn:            turnConn,
			RelayAddressGenerator: &RelayAddressGeneratorNone{Address: "1.2.3.5", Net: turnNet},
		}},
		LoggerFactory: loggerFactory,
	})
	redirectConn, err := redirectNet.ListenPacket("udp4", "1.2.3.4:3478")
	require.NoError(t, err)
	serveTryAlternate(t, redirectConn, &stun.AlternateServer{IP: net.ParseIP("1.2.3.5"), Port: 3478})

	clientConn, err := clientNet.ListenPacket("udp4", "0.0.0.0:0")
	require.NoError(t, err)
	client, err := NewClient(&ClientConfig{
		Conn:           clientConn,
		TURNServerAddr: "turn.pion.ly:3478",
		Username:       "foo",
		Password:       "pass",
		Net:            clientNet,
		LoggerFactory:  loggerFactory,
	})
	require.NoError(t, err)
	require.NoError(t, client.Listen())
	relayConn, err := client.Allocate()
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.5:3478", client.TURNServerAddr().String())

	peerConn, err := peerNet.ListenPacket("udp4", "1.2.3.7:5000")
	require.NoError(t, err)
	deadline := time.Now().Add(5 * time.Second)
	require.NoError(t, peerConn.SetReadDeadline(deadline))
	require.NoError(t, relayConn.SetReadDeadline(deadline))

	// Writing first installs the permission the peer's reply needs.
	_, err = relayConn.WriteTo([]byte("ping"), peerConn.LocalAddr())
	require.NoError(t, err)
	buf := make([]byte, 64)
	n, from, err := peerConn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "ping", string(buf[:n]))
	assert.Equal(t, relayConn.LocalAddr().String(), from.String())

	_, err = peerConn.WriteTo([]byte("pong"), from)
	require.NoError(t, err)
	n, from, err = relayConn.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(buf[:n]))
	assert.Equal(t, peerConn.LocalAddr().String(), from.String())

	assert.NoError(t, relayConn.Close())
	assert.NoError(t, peerConn.Close())
	client.Close()
	assert.NoError(t, clientConn.Close())
}
