// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"context"
	"io"
	"net"
	"runtime"
	"sync/atomic"
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			// Sleep needed for sending retransmission.
			if runtime.GOOS == "windows" {
				time.Sleep(20 * time.Millisecond)
			} else {
				time.Sleep(time.Millisecond)
			}

			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
		AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
			return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
			AuthHandler: func(ra *RequestAttributes) (key []byte, ok bool) {
				return GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
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
