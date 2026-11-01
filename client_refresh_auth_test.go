// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun/v4"
	"github.com/stretchr/testify/require"
)

// countingConn counts CreatePermission requests that reach the server.
type countingConn struct {
	net.PacketConn
	perms atomic.Int32
}

func (c *countingConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := c.PacketConn.ReadFrom(p)
	if err == nil && stun.IsMessage(p[:n]) {
		msg := &stun.Message{Raw: append([]byte{}, p[:n]...)}
		if msg.Decode() == nil && msg.Type == stun.NewType(stun.MethodCreatePermission, stun.ClassRequest) {
			c.perms.Add(1)
		}
	}

	return n, addr, err
}

// TestClientStopsRefreshingRejectedPermissions reproduces a client that keeps
// refreshing a permission the server has stopped accepting.
//
// With TURN REST credentials (coturn's use-auth-secret) the username carries an
// expiry. Once it passes, every CreatePermission refresh is rejected and no retry
// with the same credentials can succeed — yet the refresh timer keeps firing for
// the life of the allocation. In the field (coturn, 12 h credentials, connections
// that outlive them) that was one rejection every two minutes per allocation, for days.
func TestClientStopsRefreshingRejectedPermissions(t *testing.T) {
	udpListener, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	wire := &countingConn{PacketConn: udpListener}

	var valid atomic.Bool
	valid.Store(true)
	server, err := NewServer(ServerConfig{
		AuthHandler: func(ra *RequestAttributes) (userID string, key []byte, ok bool) {
			if !valid.Load() {
				return "", nil, false
			}

			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
		},
		PacketConnConfigs: []PacketConnConfig{
			{
				PacketConn: wire,
				RelayAddressGenerator: &RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP("127.0.0.1"),
					Address:      "127.0.0.1",
				},
			},
		},
		Realm: "pion.ly",
	})
	require.NoError(t, err)
	defer server.Close() //nolint:errcheck

	conn, err := net.ListenPacket("udp4", "127.0.0.1:0") // nolint: noctx
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	const every = 50 * time.Millisecond
	client, err := NewClient(&ClientConfig{
		Conn:                      conn,
		TURNServerAddr:            udpListener.LocalAddr().String(),
		Username:                  "foo",
		Password:                  "pass",
		PermissionRefreshInterval: every,
	})
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.Listen())

	allocation, err := client.Allocate()
	require.NoError(t, err)
	defer allocation.Close() //nolint:errcheck

	// One permission to refresh.
	_, err = allocation.WriteTo([]byte{0x00}, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080})
	require.NoError(t, err)
	time.Sleep(5 * every)
	require.Greater(t, wire.perms.Load(), int32(1), "the permission was never refreshed — the setup is wrong")

	// The credentials expire.
	valid.Store(false)
	before := wire.perms.Load()
	time.Sleep(40 * every)
	after := wire.perms.Load() - before
	t.Logf("CreatePermission requests after the credentials expired: %d", after)

	// A rejection is final for these credentials: one refresh may meet it (and up
	// to 3 retries), then the client must stop asking. Without the fix this is ~40.
	require.LessOrEqual(t, after, int32(4),
		"the client kept refreshing a permission the server rejects")
}
