// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/the-sarge/turn/v5/internal/proto"
	"github.com/stretchr/testify/assert"
)

// prepareHarness drives a NewUDPConn against a scripted mock TURN server.
type prepareHarness struct {
	conn      *UDPConn
	peer      *net.UDPAddr
	permCount atomic.Int32
	bindCount atomic.Int32
	bindGate  chan struct{} // If non-nil, ChannelBind transactions block on it
	permGate  chan struct{} // If non-nil, CreatePermission transactions block on it
	failPerms atomic.Bool   // If set, CreatePermission transactions return 403
	writes    struct {
		sync.Mutex
		data [][]byte
	}
}

func newPrepareHarness(t *testing.T, gateBinds bool) *prepareHarness {
	t.Helper()

	harness := &prepareHarness{
		peer: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
	}
	if gateBinds {
		harness.bindGate = make(chan struct{})
	}

	mock := &mockClient{
		performTransaction: func(msg *stun.Message, _ net.Addr, _ bool) (TransactionResult, error) {
			switch msg.Type.Method {
			case stun.MethodCreatePermission:
				harness.permCount.Add(1)
				if harness.permGate != nil {
					<-harness.permGate
				}
				if harness.failPerms.Load() {
					return TransactionResult{Msg: stun.MustBuild(
						stun.NewType(stun.MethodCreatePermission, stun.ClassErrorResponse),
						stun.ErrorCodeAttribute{Code: stun.CodeForbidden, Reason: []byte("Forbidden")},
					)}, nil
				}

				return TransactionResult{Msg: stun.MustBuild(
					stun.NewType(stun.MethodCreatePermission, stun.ClassSuccessResponse),
				)}, nil
			case stun.MethodChannelBind:
				harness.bindCount.Add(1)
				if harness.bindGate != nil {
					<-harness.bindGate
				}

				return TransactionResult{Msg: stun.MustBuild(
					stun.NewType(stun.MethodChannelBind, stun.ClassSuccessResponse),
				)}, nil
			case stun.MethodRefresh:
				return TransactionResult{}, nil
			default:
				return TransactionResult{}, errFake
			}
		},
		writeTo: func(data []byte, _ net.Addr) (int, error) {
			harness.writes.Lock()
			harness.writes.data = append(harness.writes.data, append([]byte(nil), data...))
			harness.writes.Unlock()

			return len(data), nil
		},
	}

	harness.conn = NewUDPConn(&AllocationConfig{
		Client:      mock,
		RelayedAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321},
		ServerAddr:  &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478},
		Username:    stun.NewUsername("user"),
		Realm:       stun.NewRealm("realm"),
		Integrity:   stun.NewShortTermIntegrity("pass"),
		Nonce:       stun.NewNonce("nonce"),
		Lifetime:    time.Hour,
		Log:         logging.NewDefaultLoggerFactory().NewLogger("test"),
	})
	t.Cleanup(func() { _ = harness.conn.Close() })

	return harness
}

func (harness *prepareHarness) writeCount() int {
	harness.writes.Lock()
	defer harness.writes.Unlock()

	return len(harness.writes.data)
}

func (harness *prepareHarness) lastWrite() []byte {
	harness.writes.Lock()
	defer harness.writes.Unlock()

	if len(harness.writes.data) == 0 {
		return nil
	}

	return harness.writes.data[len(harness.writes.data)-1]
}

func TestPreparePeer(t *testing.T) { //nolint:maintidx,cyclop,gocyclo
	t.Run("readiness success then ChannelData writes", func(t *testing.T) {
		harness := newPrepareHarness(t, false)

		assert.NoError(t, harness.conn.PreparePeer(context.Background(), harness.peer))
		assert.Equal(t, int32(1), harness.permCount.Load())
		assert.Equal(t, int32(1), harness.bindCount.Load())

		n, err := harness.conn.WriteTo([]byte("hello"), harness.peer)
		assert.NoError(t, err)
		assert.Equal(t, 5, n)
		assert.True(t, proto.IsChannelData(harness.lastWrite()),
			"write after successful PreparePeer must be ChannelData, not Send indication")
	})

	t.Run("invalid peers rejected", func(t *testing.T) {
		harness := newPrepareHarness(t, false)

		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(),
			&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}), errUDPAddrCast)
		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(),
			&net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 1234}), errInvalidUDPAddr)
		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(),
			&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}), errInvalidUDPAddr)
		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(),
			&net.UDPAddr{IP: net.ParseIP("224.0.0.1"), Port: 1234}), errInvalidUDPAddr)
		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(),
			&net.UDPAddr{IP: net.ParseIP("fe80::1"), Port: 1234, Zone: "en0"}), errInvalidUDPAddr)
	})

	t.Run("peer aliases share the prepared binding", func(t *testing.T) {
		harness := newPrepareHarness(t, false)
		peer := &net.UDPAddr{IP: net.ParseIP("::1"), Port: 5678}

		assert.NoError(t, harness.conn.PreparePeer(context.Background(), peer))
		assert.Equal(t, int32(1), harness.bindCount.Load())

		// A zoned alias of the prepared peer must map onto its channel binding
		// instead of bypassing it via Send indication.
		alias := &net.UDPAddr{IP: net.ParseIP("::1"), Port: 5678, Zone: "en0"}
		_, err := harness.conn.WriteTo([]byte("via alias"), alias)
		assert.NoError(t, err)
		assert.True(t, proto.IsChannelData(harness.lastWrite()),
			"write to an alias of a prepared peer must be ChannelData")
		assert.Equal(t, int32(1), harness.bindCount.Load(), "alias must not create a second binding")
	})

	t.Run("terminal failure survives an in-flight bind success", func(t *testing.T) {
		harness := newPrepareHarness(t, true)

		bound := harness.conn.bindingMgr.getOrCreate(harness.peer)
		harness.conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return harness.bindCount.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)

		// Terminalize while the bind transaction is still in flight, then let
		// it succeed: the binding must stay failed.
		bound.prepared.Store(true)
		bound.terminalize(errFake)
		close(harness.bindGate)

		assert.Eventually(t, func() bool {
			bound.muBind.Lock()
			defer bound.muBind.Unlock()

			return bound.attemptDone == nil
		}, 5*time.Second, 10*time.Millisecond)
		assert.Equal(t, bindingStateFailed, bound.state(),
			"completed bind attempt must not resurrect a terminalized binding")

		_, err := harness.conn.WriteTo([]byte("data"), harness.peer)
		assert.ErrorIs(t, err, errFake)
	})

	t.Run("same-peer callers coalesce onto one bind", func(t *testing.T) {
		harness := newPrepareHarness(t, true)

		const waiters = 4
		results := make(chan error, waiters)
		for range waiters {
			go func() {
				results <- harness.conn.PreparePeer(context.Background(), harness.peer)
			}()
		}

		// Let the first attempt start and the rest pile onto it.
		assert.Eventually(t, func() bool {
			return harness.bindCount.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)
		time.Sleep(100 * time.Millisecond)
		close(harness.bindGate)

		for range waiters {
			select {
			case err := <-results:
				assert.NoError(t, err)
			case <-time.After(5 * time.Second):
				assert.Fail(t, "timed out waiting for PreparePeer")
			}
		}
		assert.Equal(t, int32(1), harness.permCount.Load(), "permission transactions should coalesce")
		assert.Equal(t, int32(1), harness.bindCount.Load(), "ChannelBind transactions should coalesce")
	})

	t.Run("cancellation wakes only that waiter", func(t *testing.T) {
		harness := newPrepareHarness(t, true)

		ctxA, cancelA := context.WithCancelCause(context.Background())
		defer cancelA(nil)
		causeA := errors.New("waiter A gave up") //nolint:err113 // test-local cause

		resultA := make(chan error, 1)
		resultB := make(chan error, 1)
		go func() { resultA <- harness.conn.PreparePeer(ctxA, harness.peer) }()
		go func() { resultB <- harness.conn.PreparePeer(context.Background(), harness.peer) }()

		assert.Eventually(t, func() bool {
			return harness.bindCount.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)

		cancelA(causeA)
		select {
		case err := <-resultA:
			assert.ErrorIs(t, err, causeA, "canceled waiter must observe its cause")
		case <-time.After(2 * time.Second):
			assert.Fail(t, "canceled waiter did not wake promptly")
		}

		// The shared bind attempt must survive waiter A's cancellation.
		select {
		case err := <-resultB:
			assert.Failf(t, "waiter B finished early", "err: %v", err)
		case <-time.After(200 * time.Millisecond):
		}

		close(harness.bindGate)
		select {
		case err := <-resultB:
			assert.NoError(t, err, "surviving waiter should complete via the shared bind")
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for surviving waiter")
		}
		assert.Equal(t, int32(1), harness.bindCount.Load(), "cancellation must not restart or cancel the shared bind")
	})

	t.Run("cancellation wakes waiter during in-flight permission transaction", func(t *testing.T) {
		harness := newPrepareHarness(t, false)
		harness.permGate = make(chan struct{})

		// First caller's CreatePermission transaction is in flight (and holds
		// the permission mutex for its duration, as createPermission does).
		resultA := make(chan error, 1)
		go func() { resultA <- harness.conn.PreparePeer(context.Background(), harness.peer) }()
		assert.Eventually(t, func() bool {
			return harness.permCount.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)

		// A second caller for the same peer must wait on the attempt channel,
		// where its cancellation can wake it — not on the permission mutex.
		ctxB, cancelB := context.WithCancelCause(context.Background())
		defer cancelB(nil)
		resultB := make(chan error, 1)
		go func() { resultB <- harness.conn.PreparePeer(ctxB, harness.peer) }()
		time.Sleep(100 * time.Millisecond)

		cause := errors.New("waiter B gave up") //nolint:err113 // test-local cause
		cancelB(cause)
		select {
		case err := <-resultB:
			assert.ErrorIs(t, err, cause,
				"waiter must be cancelable while the permission transaction is in flight")
		case <-time.After(2 * time.Second):
			assert.Fail(t, "canceled waiter did not wake during in-flight permission transaction")
		}

		close(harness.permGate)
		select {
		case err := <-resultA:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for first caller")
		}
		assert.Equal(t, int32(1), harness.permCount.Load(), "permission transactions should coalesce")
	})

	t.Run("permission refresh failure fails writes, never Send indication", func(t *testing.T) {
		harness := newPrepareHarness(t, false)

		assert.NoError(t, harness.conn.PreparePeer(context.Background(), harness.peer))

		// Simulate the permission-refresh timer firing against a server that
		// now rejects the refresh.
		harness.failPerms.Store(true)
		harness.conn.onRefreshTimers(timerIDRefreshPerms)

		writesBefore := harness.writeCount()
		_, err := harness.conn.WriteTo([]byte("data"), harness.peer)
		assert.ErrorIs(t, err, errPermissionRefreshFailed)
		assert.Equal(t, writesBefore, harness.writeCount(),
			"failed write for a prepared peer must not emit anything (no Send indication fallback)")

		assert.ErrorIs(t, harness.conn.PreparePeer(context.Background(), harness.peer), errPermissionRefreshFailed,
			"readiness must be terminal after permission refresh failure")
	})

	t.Run("bind failure surfaces to preparing caller", func(t *testing.T) {
		harness := newPrepareHarness(t, false)

		// First permission succeeds, but every ChannelBind transaction fails.
		mock, ok := harness.conn.client.(*mockClient)
		assert.True(t, ok)
		inner := mock.performTransaction
		mock.performTransaction = func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
			if msg.Type.Method == stun.MethodChannelBind {
				harness.bindCount.Add(1)

				return TransactionResult{}, errFake
			}

			return inner(msg, to, dontWait)
		}

		err := harness.conn.PreparePeer(context.Background(), harness.peer)
		assert.ErrorIs(t, err, errChannelBindTransactionFailed)
		assert.False(t, harness.conn.isClosed())
	})

	t.Run("close joins in-flight bind workers", func(t *testing.T) {
		harness := newPrepareHarness(t, true)

		prepareResult := make(chan error, 1)
		go func() { prepareResult <- harness.conn.PreparePeer(context.Background(), harness.peer) }()

		assert.Eventually(t, func() bool {
			return harness.bindCount.Load() == 1
		}, 5*time.Second, 10*time.Millisecond)

		closeResult := make(chan error, 1)
		go func() { closeResult <- harness.conn.Close() }()

		// The waiter unblocks promptly; Close must keep waiting for the worker.
		select {
		case err := <-prepareResult:
			assert.ErrorIs(t, err, errClosed)
		case <-time.After(2 * time.Second):
			assert.Fail(t, "PreparePeer waiter did not unblock on close")
		}
		select {
		case <-closeResult:
			assert.Fail(t, "Close returned while a bind worker was still in flight")
		case <-time.After(300 * time.Millisecond):
		}

		close(harness.bindGate)
		select {
		case err := <-closeResult:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "Close did not return after the bind worker finished")
		}
	})
}
