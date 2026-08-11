// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package turn

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v5/internal/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSilentServerAllocation builds a UDP allocation whose transactions go to a
// server that never responds, driving the real transaction/retransmission
// machinery. withAbort mirrors the wiring Allocate performs.
func newSilentServerAllocation(t *testing.T, withAbort bool) *client.UDPConn {
	t.Helper()

	var listenConfig net.ListenConfig
	serverSock, err := listenConfig.ListenPacket(context.Background(), "udp4", "127.0.0.1:0") // Never responds
	require.NoError(t, err)
	clientSock, err := listenConfig.ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
	require.NoError(t, err)

	cl, err := NewClient(&ClientConfig{
		Conn:           clientSock,
		TURNServerAddr: serverSock.LocalAddr().String(),
		Username:       "user",
		Password:       "secret",
		Realm:          "realm",
		RTO:            25 * time.Millisecond,
		LoggerFactory:  logging.NewDefaultLoggerFactory(),
	})
	require.NoError(t, err)
	require.NoError(t, cl.Listen())

	config := &client.AllocationConfig{
		Client:      cl,
		RelayedAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321},
		ServerAddr:  cl.turnServerAddr,
		Username:    stun.NewUsername("user"),
		Realm:       stun.NewRealm("realm"),
		Integrity:   stun.NewShortTermIntegrity("secret"),
		Nonce:       stun.NewNonce("nonce"),
		Lifetime:    time.Hour,
		Log:         logging.NewDefaultLoggerFactory().NewLogger("test"),
	}
	if withAbort {
		config.AbortTransactions = func() {
			cl.abortPendingTransactionsTo(cl.turnServerAddr)
		}
	}

	conn := client.NewUDPConn(config)
	t.Cleanup(func() {
		_ = conn.Close()
		cl.Close()
		_ = clientSock.Close()
		_ = serverSock.Close()
	})

	return conn
}

func TestCloseInterruptsTransactionWaits(t *testing.T) {
	peer := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}

	t.Run("without abort Close waits out the retransmission budget", func(t *testing.T) {
		conn := newSilentServerAllocation(t, false)

		prepareResult := make(chan error, 1)
		go func() { prepareResult <- conn.PreparePeer(context.Background(), peer) }()
		time.Sleep(150 * time.Millisecond) // Let the CreatePermission transaction get in flight

		start := time.Now()
		assert.NoError(t, conn.Close())
		elapsed := time.Since(start)
		t.Logf("Close took %v without abort (RTO 25ms, budget ~3.2s)", elapsed)
		assert.Greater(t, elapsed, time.Second,
			"without abort, Close should block until the in-flight transaction exhausts its retransmissions")

		select {
		case err := <-prepareResult:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "PreparePeer waiter did not unblock")
		}
	})

	t.Run("with abort Close returns promptly and cancellation stays waiter-local", func(t *testing.T) {
		conn := newSilentServerAllocation(t, true)

		resultA := make(chan error, 1)
		go func() { resultA <- conn.PreparePeer(context.Background(), peer) }()

		ctxB, cancelB := context.WithCancelCause(context.Background())
		defer cancelB(nil)
		resultB := make(chan error, 1)
		go func() { resultB <- conn.PreparePeer(ctxB, peer) }()

		time.Sleep(150 * time.Millisecond) // Let the CreatePermission transaction get in flight

		// Canceling one waiter must not abort the shared transaction work.
		cause := errors.New("waiter B gave up") //nolint:err113 // test-local cause
		cancelB(cause)
		select {
		case err := <-resultB:
			assert.ErrorIs(t, err, cause)
		case <-time.After(time.Second):
			assert.Fail(t, "canceled waiter did not wake promptly")
		}
		select {
		case err := <-resultA:
			assert.Failf(t, "surviving waiter finished early", "err: %v", err)
		case <-time.After(200 * time.Millisecond):
		}

		start := time.Now()
		assert.NoError(t, conn.Close())
		elapsed := time.Since(start)
		t.Logf("Close took %v with abort", elapsed)
		assert.Less(t, elapsed, time.Second,
			"with abort, Close must not wait out the retransmission budget")

		select {
		case err := <-resultA:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "surviving waiter did not unblock on close")
		}
	})
}
