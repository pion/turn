// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package turn

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllocationDispatcher(t *testing.T) { //nolint:maintidx
	t.Run("BasicDispatch", func(t *testing.T) {
		// Create two UDP sockets to simulate PacketConn and a peer.
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		// Create dispatcher on pconn.
		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		// Register a conn for the peer's address.
		conn := dispatcher.register(peer.LocalAddr())

		// Peer sends data to pconn.
		testData := []byte("hello dispatcher")
		_, err = peer.WriteTo(testData, pconn.LocalAddr())
		require.NoError(t, err)

		// Read from conn should receive the dispatched data.
		buf := make([]byte, 100)
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, testData, buf[:n])

		// Close conn.
		assert.NoError(t, conn.Close())
	})

	t.Run("MultipleConns", func(t *testing.T) {
		// Create pconn and two peers.
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer1, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer1.Close() }()

		peer2, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer2.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		conn1 := dispatcher.register(peer1.LocalAddr())
		conn2 := dispatcher.register(peer2.LocalAddr())

		// Peer1 sends data.
		data1 := []byte("from peer1")
		_, err = peer1.WriteTo(data1, pconn.LocalAddr())
		require.NoError(t, err)

		// Peer2 sends data.
		data2 := []byte("from peer2")
		_, err = peer2.WriteTo(data2, pconn.LocalAddr())
		require.NoError(t, err)

		// Each conn should only receive its peer's data.
		buf := make([]byte, 100)

		_ = conn1.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn1.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, data1, buf[:n])

		_ = conn2.SetReadDeadline(time.Now().Add(time.Second))
		n, err = conn2.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, data2, buf[:n])

		assert.NoError(t, conn1.Close())
		assert.NoError(t, conn2.Close())
	})

	t.Run("UnknownPeerToAcceptChannel", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		knownPeer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = knownPeer.Close() }()

		unknownPeer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = unknownPeer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		// Only register knownPeer.
		conn := dispatcher.register(knownPeer.LocalAddr())

		// Unknown peer sends data - creates conn and sends to acceptCh.
		_, err = unknownPeer.WriteTo([]byte("unknown"), pconn.LocalAddr())
		require.NoError(t, err)

		// Known peer sends data.
		_, err = knownPeer.WriteTo([]byte("known"), pconn.LocalAddr())
		require.NoError(t, err)

		// Registered conn should only receive from known peer.
		buf := make([]byte, 100)
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, []byte("known"), buf[:n])

		assert.NoError(t, conn.Close())
	})

	t.Run("Accept", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		// Peer sends data without being registered - should create conn in acceptCh.
		testData := []byte("hello from unregistered peer")
		_, err = peer.WriteTo(testData, pconn.LocalAddr())
		require.NoError(t, err)

		// Accept should return the new conn.
		select {
		case acceptedConn := <-dispatcher.acceptCh:
			require.NotNil(t, acceptedConn)
			assert.Equal(t, peer.LocalAddr().String(), acceptedConn.RemoteAddr().String())

			// Read data from accepted conn.
			buf := make([]byte, 100)
			_ = acceptedConn.SetReadDeadline(time.Now().Add(time.Second))
			n, err := acceptedConn.Read(buf)
			require.NoError(t, err)
			assert.Equal(t, testData, buf[:n])

			assert.NoError(t, acceptedConn.Close())
		case <-time.After(time.Second):
			assert.Fail(t, "timeout waiting for accepted conn")
		}
	})

	t.Run("AcceptMultiplePeers", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer1, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer1.Close() }()

		peer2, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer2.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		// Both peers send data.
		_, err = peer1.WriteTo([]byte("peer1"), pconn.LocalAddr())
		require.NoError(t, err)
		_, err = peer2.WriteTo([]byte("peer2"), pconn.LocalAddr())
		require.NoError(t, err)

		// Accept both conns.
		accepted := make(map[string]*dispatchedConn)
		for i := 0; i < 2; i++ {
			select {
			case conn := <-dispatcher.acceptCh:
				accepted[conn.RemoteAddr().String()] = conn
			case <-time.After(time.Second):
				assert.Fail(t, "timeout waiting for accepted conn")
			}
		}

		// Verify both peers have conns.
		assert.Contains(t, accepted, peer1.LocalAddr().String())
		assert.Contains(t, accepted, peer2.LocalAddr().String())

		// Read from each conn.
		buf := make([]byte, 100)

		conn1 := accepted[peer1.LocalAddr().String()]
		_ = conn1.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn1.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, []byte("peer1"), buf[:n])

		conn2 := accepted[peer2.LocalAddr().String()]
		_ = conn2.SetReadDeadline(time.Now().Add(time.Second))
		n, err = conn2.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, []byte("peer2"), buf[:n])

		assert.NoError(t, conn1.Close())
		assert.NoError(t, conn2.Close())
	})

	t.Run("Read", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		// Peer sends multiple packets.
		_, err = peer.WriteTo([]byte("packet1"), pconn.LocalAddr())
		require.NoError(t, err)
		_, err = peer.WriteTo([]byte("packet2"), pconn.LocalAddr())
		require.NoError(t, err)

		// Read both packets in order.
		buf := make([]byte, 100)
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))

		n, err := conn.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, []byte("packet1"), buf[:n])

		n, err = conn.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, []byte("packet2"), buf[:n])

		assert.NoError(t, conn.Close())
	})

	t.Run("AcceptedConnCanReply", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))

		// Peer sends data to trigger accept.
		_, err = peer.WriteTo([]byte("hello"), pconn.LocalAddr())
		require.NoError(t, err)

		// Accept the conn.
		var acceptedConn *dispatchedConn
		select {
		case acceptedConn = <-dispatcher.acceptCh:
		case <-time.After(time.Second):
			assert.Fail(t, "timeout waiting for accepted conn")
		}

		// Accepted conn can write back to peer.
		replyData := []byte("reply from server")
		n, err := acceptedConn.Write(replyData)
		require.NoError(t, err)
		assert.Equal(t, len(replyData), n)

		// Peer receives reply.
		buf := make([]byte, 100)
		_ = peer.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := peer.ReadFrom(buf)
		require.NoError(t, err)
		assert.Equal(t, replyData, buf[:n])
		assert.Equal(t, pconn.LocalAddr().String(), from.String())

		assert.NoError(t, acceptedConn.Close())
	})
}

func TestAllocationDispatchedConn(t *testing.T) {
	t.Run("Write", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		// Write to conn should send to peer.
		testData := []byte("outgoing data")
		n, err := conn.Write(testData)
		require.NoError(t, err)
		assert.Equal(t, len(testData), n)

		// Peer should receive the data.
		buf := make([]byte, 100)
		_ = peer.SetReadDeadline(time.Now().Add(time.Second))
		n, from, err := peer.ReadFrom(buf)
		require.NoError(t, err)
		assert.Equal(t, testData, buf[:n])
		assert.Equal(t, pconn.LocalAddr().String(), from.String())

		assert.NoError(t, conn.Close())
	})

	t.Run("LocalAddr", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		assert.Equal(t, pconn.LocalAddr(), conn.LocalAddr())
		assert.NoError(t, conn.Close())
	})

	t.Run("RemoteAddr", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		assert.Equal(t, peer.LocalAddr().String(), conn.RemoteAddr().String())
		assert.NoError(t, conn.Close())
	})

	t.Run("DoubleClose", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		// Double close should not panic.
		assert.NoError(t, conn.Close())
		assert.NoError(t, conn.Close())
	})

	t.Run("ReadDeadline", func(t *testing.T) {
		pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = pconn.Close() }()

		peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
		require.NoError(t, err)
		defer func() { _ = peer.Close() }()

		dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
		conn := dispatcher.register(peer.LocalAddr())

		// Set a short deadline and try to read (no data sent).
		_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		buf := make([]byte, 100)
		_, err = conn.Read(buf)
		assert.Error(t, err, "read should timeout")

		assert.NoError(t, conn.Close())
	})
}

func TestAllocationDispatcherUnregister(t *testing.T) {
	pconn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer func() { _ = pconn.Close() }()

	peer, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer func() { _ = peer.Close() }()

	dispatcher := newPacketDispatcher(pconn, make(chan struct{}))
	conn := dispatcher.register(peer.LocalAddr())

	// Verify conn is registered.
	dispatcher.mu.Lock()
	_, exists := dispatcher.conns[peer.LocalAddr().String()]
	dispatcher.mu.Unlock()
	assert.True(t, exists, "conn should be registered")

	// Close conn (which unregisters).
	assert.NoError(t, conn.Close())

	// Verify conn is unregistered.
	dispatcher.mu.Lock()
	_, exists = dispatcher.conns[peer.LocalAddr().String()]
	dispatcher.mu.Unlock()
	assert.False(t, exists, "conn should be unregistered after close")
}

func TestAllocationDialUDP(t *testing.T) {
	// Create a TURN server.
	udpListener, err := net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)

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
	require.NoError(t, err)
	defer func() { _ = server.Close() }()

	serverAddr := udpListener.LocalAddr().String()

	// Create a sink to receive data.
	sink, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer func() { _ = sink.Close() }()

	// Create connection to TURN server.
	turnConn, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)

	// Create dialer.
	dialer, err := NewAllocation("udp", &ClientConfig{
		Conn:           turnConn,
		TURNServerAddr: serverAddr,
		Username:       "user",
		Password:       "pass",
		Realm:          "pion.ly",
	})
	require.NoError(t, err)

	// TCP dialer should fail.
	_, err = dialer.Dial("tcp", "127.0.0.1:0")
	require.Error(t, err)

	// Dial the sink.
	conn, err := dialer.Dial("udp", sink.LocalAddr().String())
	require.NoError(t, err)

	// Addr() should return the relay address.
	relayAddr := dialer.Addr()
	require.NotNil(t, relayAddr)
	assert.Equal(t, conn.LocalAddr().String(), relayAddr.String())

	// Write data through TURN.
	testData := []byte("hello via TURN UDP")
	_, err = conn.Write(testData)
	require.NoError(t, err)

	// Read at sink.
	buf := make([]byte, 100)
	_ = sink.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _, err := sink.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	// Cleanup.
	assert.NoError(t, conn.Close())
	assert.NoError(t, dialer.Close())
	_ = turnConn.Close()
}

func TestAllocationDialTCP(t *testing.T) {
	// Create a TURN server with TCP listener.
	tcpListener, err := net.Listen("tcp4", "127.0.0.1:0") //nolint:noctx
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
	defer func() { _ = server.Close() }()

	serverAddr := tcpListener.Addr().String()

	// Create a UDP sink.
	sink, err := net.ListenPacket("udp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer func() { _ = sink.Close() }()

	// Create a TCP sink to receive connections.
	sinkListener, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer func() { _ = sinkListener.Close() }()

	// Accept in background.
	sinkConnCh := make(chan net.Conn, 1)
	go func() {
		conn, listenErr := sinkListener.Accept()
		if listenErr == nil {
			sinkConnCh <- conn
		}
	}()

	// Create connection to TURN server.
	turnConn, err := net.Dial("tcp", serverAddr) //nolint:noctx
	require.NoError(t, err)

	// Create dialer.
	dialer, err := NewAllocation("tcp", &ClientConfig{
		Conn:           NewSTUNConn(turnConn),
		TURNServerAddr: serverAddr,
		Username:       "user",
		Password:       "pass",
		Realm:          "pion.ly",
	})
	require.NoError(t, err)

	// A TCP allocation should fail when dialing UDP.
	_, err = dialer.Dial("udp", sink.LocalAddr().String())
	require.Error(t, err)

	// Dial the TCP sink.
	conn, err := dialer.Dial("tcp", sinkListener.Addr().String())
	require.NoError(t, err)

	// Wait for sink to accept.
	var sinkConn net.Conn
	select {
	case sinkConn = <-sinkConnCh:
	case <-time.After(5 * time.Second):
		assert.Fail(t, "timeout waiting for accepted conn")
	}
	defer func() { _ = sinkConn.Close() }()

	// Addr() should return the relay address.
	relayAddr := dialer.Addr()
	require.NotNil(t, relayAddr)
	assert.Equal(t, conn.LocalAddr().String(), relayAddr.String())

	// Write data through TURN.
	testData := []byte("hello via TURN TCP")
	_, err = conn.Write(testData)
	require.NoError(t, err)

	// Read at sink.
	buf := make([]byte, 100)
	_ = sinkConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := sinkConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, testData, buf[:n])

	// Cleanup.
	assert.NoError(t, conn.Close())
	assert.NoError(t, dialer.Close())
	_ = turnConn.Close()
}
