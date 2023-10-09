// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build offloadxdp && !js

package offload

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v3/internal/proto"
	"github.com/stretchr/testify/assert"
)

// TestXDPOffload executes XDP offload unit tests on localhost
//
// The tests include forwarding clients ChannelData traffic to peer, and peer UDP traffic to a client.
//
// The test setup on localhost exercising the 'lo' interface:
// client (port 2000) <-> turn listen (3478) <- XDP offload -> turn relay (4000) <-> peer (5000)
//
// The loopback interface tends to generate incorrect UDP checksums, which is fine in most cases.
// However, the XDP offload incrementally updates this checksum resulting errnous checksum on the
// receiver side. This leads to packet drops. To mitigate this issue, tests use raw sockets to send
// pre-crafted packets having correct checksums.
//
// Note: Running these tests require privileged users (e.g., root) to run the XDP offload.
func TestXDPOffload(t *testing.T) {
	// timeout of connnection read deadlines
	const readTimeoutInterval = 500 * time.Millisecond

	clientAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:2000")
	assert.NoError(t, err)
	turnListenAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)
	turnRelayAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:4000")
	assert.NoError(t, err)
	peerAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:5000")
	assert.NoError(t, err)

	// IP addrs for raw sockets
	clientIPAddr, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	assert.NoError(t, err)
	turnListenIPAddr, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	assert.NoError(t, err)
	turnRelayIPAddr, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	assert.NoError(t, err)
	peerIPAddr, err := net.ResolveIPAddr("ip4", "127.0.0.1")
	assert.NoError(t, err)

	// Channel Data Header (channel number: 0x4000, message length: 7)
	chanDataHdr := []byte{0x40, 0x00, 0x00, 0x07}
	// payload is the string 'payload'
	payload := []byte{0x70, 0x61, 0x79, 0x6c, 0x6f, 0x61, 0x64}

	// ChannelData header + 'payload' + padding
	chanDataRecv := append(append(chanDataHdr, payload...), 0x00)

	// a UDP header + ChannelData header + 'payload' + padding
	chanDataSent := append(
		[]byte{0x07, 0xd0, 0x0d, 0x96, 0x00, 0x14, 0xef, 0x26},
		append(append(chanDataHdr, payload...), 0x00)...)

	// UDP Data is 'payload'
	udpRecv := payload

	// a UDP header + 'payload'
	udpSent := append(
		[]byte{0x13, 0x88, 0x0f, 0xa0, 0x00, 0x0f, 0x21, 0x76},
		payload...)

	// setup client side
	clientIPConn, err := net.ListenIP("ip4:udp", clientIPAddr)
	assert.NoError(t, err, "cannot create client connection")
	defer clientIPConn.Close() //nolint:errcheck

	// setup peer side
	peerIPConn, err := net.ListenIP("ip4:udp", peerIPAddr)
	assert.NoError(t, err, "cannot create peer connection")
	defer peerIPConn.Close() //nolint:errcheck

	clientConn, err := net.ListenUDP("udp", clientAddr)
	assert.NoError(t, err, "cannot create client connection")
	defer clientConn.Close() //nolint:errcheck

	peerConn, err := net.ListenUDP("udp", peerAddr)
	assert.NoError(t, err, "cannot create peer connection")
	defer peerConn.Close() //nolint:errcheck

	// init offload
	client := Connection{
		LocalAddr:  turnListenAddr,
		RemoteAddr: clientAddr,
		Protocol:   proto.ProtoUDP,
		ChannelID:  0x4000,
	}
	peer := Connection{
		LocalAddr:  turnRelayAddr,
		RemoteAddr: peerAddr,
		Protocol:   proto.ProtoUDP,
	}

	loggerFactory := logging.NewDefaultLoggerFactory()
	logger := loggerFactory.NewLogger("xdp-test")
	lo, err := net.InterfaceByName("lo")
	assert.NoError(t, err, "no interface: 'lo'")

	xdpEngine, err := NewXdpEngine([]net.Interface{*lo}, logger)
	assert.NoError(t, err, "cannot instantiate XDP offload engine")
	defer xdpEngine.Shutdown()

	err = xdpEngine.Init()
	assert.NoError(t, err, "cannot init XDP offload engine")

	t.Run("upsert/remove entries of the eBPF maps", func(t *testing.T) {
		c, err := xdpEngine.List()
		assert.NoError(t, err, "cannot list XDP offload maps")
		assert.Equal(t, 0, len(c), "map should be empty at start")

		assert.NoError(t, xdpEngine.Upsert(client, peer), "cannot upsert new offload")

		c, err = xdpEngine.List()
		assert.NoError(t, err, "cannot list XDP offload maps")
		assert.Equal(t, 3, len(c),
			"maps should have three elements (1 upstream and 2 downstreams)")

		assert.NoError(t,
			xdpEngine.Remove(client, peer),
			"error in removing peer connection")

		assert.Error(t,
			xdpEngine.Remove(client, peer),
			"error in removing non-existing peer connection")
	})

	t.Run("pass packet from peer to client", func(t *testing.T) {
		assert.NoError(t, xdpEngine.Upsert(client, peer), "cannot upsert new offload")

		received := make([]byte, len(chanDataRecv))

		_, err := peerIPConn.WriteToIP(udpSent, turnRelayIPAddr)
		assert.NoError(t, err, "error in sending peer data")

		err = clientConn.SetReadDeadline(time.Now().Add(readTimeoutInterval))
		assert.NoError(t, err)
		n, addr, err := clientConn.ReadFromUDP(received)
		assert.NoError(t, err, "error in receiving data on client side")

		assert.Equal(t, chanDataRecv, received, "expect match")
		assert.True(t, proto.IsChannelData(received), "expect channel data")
		assert.Equal(t, len(chanDataRecv), n, "expect payload length padded to align 4B")
		assert.Equal(t, turnListenAddr.String(), addr.String(), "expect server listener address")
	})

	t.Run("pass packet from client to peer", func(t *testing.T) {
		assert.NoError(t, xdpEngine.Upsert(client, peer), "cannot upsert new offload")

		received := make([]byte, len(udpRecv))

		_, err := clientIPConn.WriteToIP(chanDataSent, turnListenIPAddr)
		assert.NoError(t, err, "error in sending channel data")

		err = peerConn.SetReadDeadline(time.Now().Add(readTimeoutInterval))
		assert.NoError(t, err)
		n, addr, err := peerConn.ReadFromUDP(received)
		assert.NoError(t, err, "error in receiving data on client side")

		assert.Equal(t, udpRecv, received, "expect match")
		assert.Equal(t, len(payload), n, "expect payload length")
		assert.Equal(t, turnRelayAddr.String(), addr.String(), "expect server relay address")
	})
}
