// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package turn

import (
	"context"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/dtls/v4"
	"github.com/pion/logging"
	"github.com/pion/transport/v5/test"
	"github.com/pion/turn/v6/internal/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type channelObservingConn struct {
	net.PacketConn
	channel atomic.Uint32
}

func (c *channelObservingConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if proto.IsChannelData(p) {
		c.channel.CompareAndSwap(0, uint32(binary.BigEndian.Uint16(p)))
	}

	return c.PacketConn.WriteTo(p, addr)
}

func newDTLSTestServer(t *testing.T) (*Server, net.Listener, *x509.CertPool) {
	t.Helper()

	ca, key, err := generateCACert(t)
	require.NoError(t, err)
	cert, err := generateSignedCert(t, "server", ca, key)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	udp, err := net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	listener, err := dtls.Listen(udp, dtls.WithCertificates(cert))
	require.NoError(t, err)

	server, err := NewServer(ServerConfig{
		Realm: "pion.ly",
		AuthHandler: func(ra *RequestAttributes) (string, []byte, bool) {
			if ra.DTLS == nil || ra.TLS != nil {
				return "", nil, false
			}

			return ra.Username, GenerateAuthKey(ra.Username, ra.Realm, "pass"), true
		},
		ListenerConfigs: []ListenerConfig{{
			Listener: listener,
			RelayAddressGenerator: &RelayAddressGeneratorStatic{
				RelayAddress: net.ParseIP("127.0.0.1"),
				Address:      "127.0.0.1",
			},
		}},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = server.Close() })

	return server, listener, roots
}

func TestServerDTLS(t *testing.T) {
	t.Cleanup(test.CheckRoutines(t))
	defer test.TimeOut(10 * time.Second).Stop()

	server, listener, roots := newDTLSTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := DialDTLS(ctx, "udp4", listener.Addr().String())
	require.Error(t, err)
	conn, err := DialDTLS(ctx, "udp4", listener.Addr().String(), dtls.WithRootCAs(roots))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	observed := &channelObservingConn{PacketConn: conn}
	client, err := NewClient(&ClientConfig{
		STUNServerAddr: listener.Addr().String(),
		TURNServerAddr: listener.Addr().String(),
		Conn:           observed,
		Username:       "user",
		Password:       "pass",
		Realm:          "pion.ly",
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	require.NoError(t, err)
	t.Cleanup(client.Close)
	require.NoError(t, client.Listen())
	relay, err := client.Allocate()
	require.NoError(t, err)
	t.Cleanup(func() { _ = relay.Close() })

	peer, err := net.ListenPacket("udp4", "127.0.0.1:0") //nolint:noctx
	require.NoError(t, err)
	defer peer.Close() //nolint:errcheck
	buf := make([]byte, 1500)
	for observed.channel.Load() == 0 {
		_, err = relay.WriteTo([]byte("ping"), peer.LocalAddr())
		require.NoError(t, err)
		require.NoError(t, peer.SetReadDeadline(time.Now().Add(time.Second)))
		n, _, readErr := peer.ReadFrom(buf)
		require.NoError(t, readErr)
		assert.Equal(t, "ping", string(buf[:n]))
	}

	_, err = peer.WriteTo([]byte("pong"), relay.LocalAddr())
	require.NoError(t, err)
	require.NoError(t, relay.SetReadDeadline(time.Now().Add(time.Second)))
	n, _, err := relay.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(buf[:n]))

	message := []byte{0, 0, 0, 3, 'o', 'd', 'd'}
	binary.BigEndian.PutUint16(message, uint16(observed.channel.Load())) //nolint:gosec
	_, err = client.WriteTo(message, listener.Addr())
	require.NoError(t, err)
	n, _, err = peer.ReadFrom(buf)
	require.NoError(t, err)
	assert.Equal(t, "odd", string(buf[:n]))
	_, err = client.SendBindingRequest()
	require.NoError(t, err)
	require.NoError(t, relay.Close())
	assert.Eventually(t, func() bool { return server.AllocationCount() == 0 }, time.Second, time.Millisecond)
	client.Close()
	require.NoError(t, listener.Close())
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err = conn.ReadFrom(buf)
	assert.ErrorIs(t, err, io.EOF)
}
