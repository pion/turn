// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package offload

import (
	"net"
	"testing"

	"github.com/pion/logging"
	"github.com/pion/turn/v3/internal/proto"
	"github.com/stretchr/testify/assert"
)

// TestNullOffload executes Null offload unit tests
func TestNullOffload(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()
	logger := loggerFactory.NewLogger("null-test")

	nullEngine, err := NewNullEngine(logger)
	assert.NoError(t, err, "cannot instantiate Null offload engine")
	defer nullEngine.Shutdown()

	err = nullEngine.Init()
	assert.NoError(t, err, "cannot init Null offload engine")

	clientAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2000}
	turnListenAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3478}
	turnRelayAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4000}
	peerAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}

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

	t.Run("remove from conntrack map", func(t *testing.T) {
		assert.NoError(t,
			nullEngine.Upsert(client, peer),
			"error in upserting client connection")

		assert.NoError(t,
			nullEngine.Upsert(peer, client),
			"error in upserting peer connection")

		assert.NoError(t,
			nullEngine.Remove(client, peer),
			"error in removing client connection")

		assert.Error(t,
			nullEngine.Remove(client, peer),
			"error in removing non-existing client connection")

		assert.NoError(t,
			nullEngine.Remove(peer, client),
			"error in removing peer connection")

		assert.Error(t,
			nullEngine.Remove(peer, client),
			"error in removing non-existing peer connection")
	})

	t.Run("upsert/remove entries of the conntrack map", func(t *testing.T) {
		ct, _ := nullEngine.List()
		assert.Equal(t, 0, len(ct), "map should be empty at start")

		assert.NoError(t,
			nullEngine.Upsert(client, peer),
			"error in upserting client connection")

		assert.NoError(t,
			nullEngine.Upsert(peer, client),
			"error in upserting peer connection")

		ct, _ = nullEngine.List()
		assert.Equal(t, 2, len(ct), "map should have two elements")

		assert.NoError(t,
			nullEngine.Remove(client, peer),
			"error in removing non-existing client connection")

		ct, _ = nullEngine.List()
		assert.Equal(t, 1, len(ct), "map should have two elements")
	})
}
