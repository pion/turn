// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v5/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestUDPConn(t *testing.T) { // nolint:maintidx,cyclop,gocyclo
	makeConn := func(client *mockClient, bm *bindingManager) UDPConn {
		return UDPConn{
			allocation: allocation{
				client: client,
				log:    logging.NewDefaultLoggerFactory().NewLogger("test"),
			},
			bindingMgr:             bm,
			bindingRefreshInterval: defaultBindingRefreshInterval,
		}
	}

	staleNonceMsg := func() *stun.Message {
		return stun.MustBuild(
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			stun.CodeStaleNonce,
			stun.NewNonce("new-nonce-123"),
		)
	}

	badRequestMsg := func() *stun.Message {
		return stun.MustBuild(
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			stun.ErrorCodeAttribute{Code: stun.CodeBadRequest, Reason: []byte("Bad Request")},
		)
	}

	t.Run("maybeBind()", func(t *testing.T) {
		tests := []struct {
			name          string
			initialState  bindingState
			interimState  bindingState
			finalState    bindingState
			pastInterval  bool
			shouldSucceed bool
		}{
			{"idle -> request -> ready", bindingStateIdle, bindingStateRequest, bindingStateReady, false, true},
			{"idle -> request -> failed", bindingStateIdle, bindingStateRequest, bindingStateFailed, false, false},
			{"unknown -> request -> ready", bindingStateUnknown, bindingStateRequest, bindingStateReady, false, true},
			{"ready unknown -> refresh -> ready", bindingStateReadyUnknown, bindingStateRefresh, bindingStateReady, false, true},
			{"ready (stale) -> refresh -> ready", bindingStateReady, bindingStateRefresh, bindingStateReady, true, true},
			{"ready (stale) -> refresh -> failed", bindingStateReady, bindingStateRefresh, bindingStateFailed, true, false},

			// Noop cases:
			{"ready (noop)", bindingStateReady, bindingStateReady, bindingStateReady, false, true},
			{"request (noop)", bindingStateRequest, bindingStateRequest, bindingStateRequest, false, true},
			{"refresh (noop)", bindingStateRefresh, bindingStateRefresh, bindingStateRefresh, false, true},
			{"failed (noop)", bindingStateFailed, bindingStateFailed, bindingStateFailed, false, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				unblock := make(chan struct{})

				bm := newBindingManager()
				bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
				conn := makeConn(&mockClient{
					performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
						<-unblock
						if tt.shouldSucceed {
							return TransactionResult{Msg: new(stun.Message)}, nil
						}

						return TransactionResult{Msg: staleNonceMsg()}, nil
					},
				}, bm)

				bound.setState(tt.initialState)
				if tt.pastInterval {
					bound.setRefreshedAt(time.Now().Add(-(defaultBindingRefreshInterval + 1*time.Minute)))
				}

				conn.maybeBind(bound)
				assert.Equal(t, tt.interimState, bound.state())

				// Release barrier so inner bind() can move forward.
				close(unblock)

				assert.Eventually(t, func() bool {
					return bound.state() == tt.finalState
				}, 5*time.Second, 10*time.Millisecond)
			})
		}
	})

	t.Run("bind()", func(t *testing.T) {
		tests := []struct {
			name                 string
			transactionFn        func(*stun.Message, net.Addr, bool) (TransactionResult, error)
			expectErr            error
			expectErrContains    string
			expectBadRequest     bool
			expectBindingDeleted bool
			expectNonceChanged   bool
		}{
			{
				name: "PerformTransaction returns error",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					return TransactionResult{}, errFake
				},
				expectErr:            errFake,
				expectBindingDeleted: false,
			},
			{
				name: "ErrorResponse with CodeStaleNonce triggers nonce update",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					return TransactionResult{Msg: staleNonceMsg()}, nil
				},
				expectErr:          errTryAgain,
				expectNonceChanged: true,
			},
			{
				name: "ErrorResponse with error code returns cannot bind channel error",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					res := stun.MustBuild(
						stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
						stun.ErrorCodeAttribute{Code: stun.CodeForbidden, Reason: []byte("Forbidden")},
					)

					return TransactionResult{Msg: res}, nil
				},
				expectErr:         errCannotBindChannel,
				expectErrContains: "received error",
			},
			{
				name: "ErrorResponse with CodeBadRequest is detectable",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					res := stun.MustBuild(
						stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
						stun.ErrorCodeAttribute{Code: stun.CodeBadRequest, Reason: []byte("Bad Request")},
					)

					return TransactionResult{Msg: res}, nil
				},
				expectErr:         errCannotBindChannel,
				expectErrContains: "received error",
				expectBadRequest:  true,
			},
			{
				name: "ErrorResponse without error code returns unexpected response type error",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					res := stun.MustBuild(
						stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
					)

					return TransactionResult{Msg: res}, nil
				},
				expectErr:         errCannotBindChannel,
				expectErrContains: "unexpected response type",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				bm := newBindingManager()
				bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
				conn := makeConn(&mockClient{performTransaction: tt.transactionFn}, bm)

				nonceT0 := conn.nonce()

				err := conn.bind(bound)
				if tt.expectErr == nil {
					assert.NoError(t, err)
				} else {
					assert.ErrorIs(t, err, tt.expectErr)
				}
				if tt.expectErrContains != "" {
					assert.ErrorContains(t, err, tt.expectErrContains)
				}
				assert.Equal(t, tt.expectBadRequest, errors.Is(err, errChannelBindBadRequest))

				if tt.expectBindingDeleted {
					assert.Empty(t, bm.chanMap)
					assert.Empty(t, bm.addrMap)
				} else {
					// Binding should remain so we don't re-bind the same peer with a different channel number
					// after a lost/failed ChannelBind transaction.
					assert.NotEmpty(t, bm.chanMap)
					assert.NotEmpty(t, bm.addrMap)
					b2, ok := bm.findByAddr(bound.addr)
					assert.True(t, ok)
					assert.Equal(t, bound.number, b2.number)
				}

				nonceT1 := conn.nonce()
				if tt.expectNonceChanged {
					assert.NotEqual(t, nonceT0, nonceT1, "should change")
					assert.NotEmpty(t, nonceT1, "should be non-empty")
				} else {
					assert.Equal(t, nonceT0, nonceT1, "should remain unchanged")
				}
			})
		}
	})

	t.Run("maybeBind() retries unknown binding after transaction failure", func(t *testing.T) {
		var failed atomic.Bool

		bm := newBindingManager()
		bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
		originalCh := bound.number
		conn := makeConn(&mockClient{
			performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
				if failed.CompareAndSwap(false, true) {
					return TransactionResult{}, errFake
				}

				return TransactionResult{Msg: new(stun.Message)}, nil
			},
		}, bm)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateUnknown
		}, 5*time.Second, 10*time.Millisecond)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateReady
		}, 5*time.Second, 10*time.Millisecond)

		b2, ok := bm.findByAddr(bound.addr)
		assert.True(t, ok)
		assert.Equal(t, originalCh, b2.number)
	})

	t.Run("ChannelBind 400 closes allocation", func(t *testing.T) {
		relayedAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
		peerAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000}
		serverAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478}

		deallocatedCh := make(chan net.Addr, 1)
		refreshLifetimeCh := make(chan time.Duration, 1)
		refreshDontWaitCh := make(chan bool, 1)
		refreshErrCh := make(chan error, 1)

		client := &mockClient{
			performTransaction: func(msg *stun.Message, _ net.Addr, dontWait bool) (TransactionResult, error) {
				switch msg.Type.Method {
				case stun.MethodChannelBind:
					return TransactionResult{
						Msg: stun.MustBuild(
							stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
							stun.ErrorCodeAttribute{Code: stun.CodeBadRequest, Reason: []byte("Bad Request")},
						),
					}, nil
				case stun.MethodRefresh:
					var lifetime proto.Lifetime
					if err := lifetime.GetFrom(msg); err != nil {
						refreshErrCh <- err
					} else {
						refreshLifetimeCh <- lifetime.Duration
						refreshDontWaitCh <- dontWait
					}

					return TransactionResult{}, nil
				default:
					return TransactionResult{}, errFake
				}
			},
			onDeallocated: func(addr net.Addr) {
				deallocatedCh <- addr
			},
		}

		conn := NewUDPConn(&AllocationConfig{
			Client:      client,
			RelayedAddr: relayedAddr,
			ServerAddr:  serverAddr,
			Username:    stun.NewUsername("user"),
			Realm:       stun.NewRealm("realm"),
			Integrity:   stun.NewShortTermIntegrity("pass"),
			Nonce:       stun.NewNonce("nonce"),
			Lifetime:    time.Hour,
			Log:         logging.NewDefaultLoggerFactory().NewLogger("test"),
		})
		defer func() { _ = conn.Close() }()

		bound := conn.bindingMgr.create(peerAddr)
		conn.maybeBind(bound)

		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateFailed && conn.isClosed()
		}, 5*time.Second, 10*time.Millisecond)

		select {
		case err := <-refreshErrCh:
			assert.NoError(t, err)
		default:
		}
		select {
		case deallocatedAddr := <-deallocatedCh:
			assert.Equal(t, relayedAddr, deallocatedAddr)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for deallocation callback")
		}

		select {
		case lifetime := <-refreshLifetimeCh:
			assert.Equal(t, time.Duration(0), lifetime)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for refresh deallocation")
		}

		select {
		case dontWait := <-refreshDontWaitCh:
			assert.True(t, dontWait)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for refresh dontWait flag")
		}

		_, err := conn.WriteTo([]byte("still closed"), peerAddr)
		assert.ErrorIs(t, err, errClosed)
	})

	t.Run("ChannelBind 400 after unknown binding closes allocation", func(t *testing.T) {
		relayedAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
		peerAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		serverAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478}

		var channelBindAttempts atomic.Int32
		deallocatedCh := make(chan net.Addr, 1)

		conn := NewUDPConn(&AllocationConfig{
			Client: &mockClient{
				performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
					switch msg.Type.Method {
					case stun.MethodChannelBind:
						if channelBindAttempts.Add(1) == 1 {
							return TransactionResult{}, errFake
						}

						return TransactionResult{Msg: badRequestMsg()}, nil
					case stun.MethodRefresh:
						return TransactionResult{}, nil
					default:
						return TransactionResult{}, errFake
					}
				},
				onDeallocated: func(addr net.Addr) {
					deallocatedCh <- addr
				},
			},
			RelayedAddr: relayedAddr,
			ServerAddr:  serverAddr,
			Username:    stun.NewUsername("user"),
			Realm:       stun.NewRealm("realm"),
			Integrity:   stun.NewShortTermIntegrity("pass"),
			Nonce:       stun.NewNonce("nonce"),
			Lifetime:    time.Hour,
			Log:         logging.NewDefaultLoggerFactory().NewLogger("test"),
		})
		defer func() { _ = conn.Close() }()

		bound := conn.bindingMgr.create(peerAddr)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateUnknown
		}, 5*time.Second, 10*time.Millisecond)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateFailed && conn.isClosed()
		}, 5*time.Second, 10*time.Millisecond)
		assert.Equal(t, int32(2), channelBindAttempts.Load())

		select {
		case deallocatedAddr := <-deallocatedCh:
			assert.Equal(t, relayedAddr, deallocatedAddr)
		case <-time.After(5 * time.Second):
			assert.Fail(t, "timed out waiting for deallocation callback")
		}
	})

	t.Run("ChannelBind 400 after lost ready refresh keeps saved binding", func(t *testing.T) {
		relayedAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
		peerAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
		serverAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478}

		var channelBindAttempts atomic.Int32

		conn := NewUDPConn(&AllocationConfig{
			Client: &mockClient{
				performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
					switch msg.Type.Method {
					case stun.MethodChannelBind:
						if channelBindAttempts.Add(1) == 1 {
							return TransactionResult{}, newTimeoutError("channel bind timeout")
						}

						return TransactionResult{Msg: badRequestMsg()}, nil
					case stun.MethodRefresh:
						return TransactionResult{}, nil
					default:
						return TransactionResult{}, errFake
					}
				},
			},
			RelayedAddr: relayedAddr,
			ServerAddr:  serverAddr,
			Username:    stun.NewUsername("user"),
			Realm:       stun.NewRealm("realm"),
			Integrity:   stun.NewShortTermIntegrity("pass"),
			Nonce:       stun.NewNonce("nonce"),
			Lifetime:    time.Hour,
			Log:         logging.NewDefaultLoggerFactory().NewLogger("test"),
		})
		defer func() { _ = conn.Close() }()

		bound := conn.bindingMgr.create(peerAddr)
		staleRefreshedAt := time.Now().Add(-(defaultBindingRefreshInterval + time.Minute))
		bound.setState(bindingStateReady)
		bound.setRefreshedAt(staleRefreshedAt)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return bound.state() == bindingStateReadyUnknown
		}, 5*time.Second, 10*time.Millisecond)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return channelBindAttempts.Load() == 2 && bound.state() == bindingStateReady
		}, 5*time.Second, 10*time.Millisecond)
		assert.True(t, bound.refreshedAt().Equal(staleRefreshedAt))
		assert.False(t, conn.isClosed())
	})

	t.Run("ChannelBind 400 refresh keeps saved binding", func(t *testing.T) {
		bm := newBindingManager()
		bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
		staleRefreshedAt := time.Now().Add(-(defaultBindingRefreshInterval + time.Minute))
		var channelBindAttempts atomic.Int32
		bound.setState(bindingStateReady)
		bound.setRefreshedAt(staleRefreshedAt)
		conn := makeConn(&mockClient{
			performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
				channelBindAttempts.Add(1)

				return TransactionResult{Msg: badRequestMsg()}, nil
			},
		}, bm)

		conn.maybeBind(bound)
		assert.Eventually(t, func() bool {
			return channelBindAttempts.Load() == 1 && bound.state() == bindingStateReady
		}, 5*time.Second, 10*time.Millisecond)
		assert.True(t, bound.refreshedAt().Equal(staleRefreshedAt))
		startState, ok := conn.startBinding(bound)
		assert.True(t, ok, "recovered binding should remain eligible for refresh")
		assert.Equal(t, bindingStateReady, startState)
		assert.Equal(t, bindingStateRefresh, bound.state())
		assert.False(t, conn.isClosed())
	})

	t.Run("WriteTo()", func(t *testing.T) {
		client := &mockClient{
			performTransaction: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
				return TransactionResult{}, errFake
			},
			writeTo: func(data []byte, _ net.Addr) (int, error) {
				return len(data), nil
			},
		}

		addr := &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 1234,
		}

		pm := newPermissionMap()
		assert.True(t, pm.insert(addr, &permission{
			st: permStatePermitted,
		}))

		bm := newBindingManager()
		binding := bm.create(addr)
		binding.setState(bindingStateReady)

		conn := UDPConn{
			allocation: allocation{
				client:  client,
				permMap: pm,
			},
			bindingMgr: bm,
		}

		buf := []byte("Hello")
		n, err := conn.WriteTo(buf, addr)
		assert.NoError(t, err, "should fail")
		assert.Equal(t, len(buf), n)
	})

	t.Run("WriteTo() returns real payload length", func(t *testing.T) {
		var writtenData []byte
		client := &mockClient{
			performTransaction: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
				// Return success for CreatePermission.
				return TransactionResult{
					Msg: stun.MustBuild(stun.NewType(stun.MethodCreatePermission, stun.ClassSuccessResponse)),
				}, nil
			},
			writeTo: func(data []byte, _ net.Addr) (int, error) {
				writtenData = data
				// Return the actual number of bytes written (the STUN message size).
				return len(data), nil
			},
		}

		addr := &net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 1234,
		}

		conn := UDPConn{
			allocation: allocation{
				client:     client,
				serverAddr: &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478},
				permMap:    newPermissionMap(),
				username:   stun.NewUsername("user"),
				realm:      stun.NewRealm("realm"),
				integrity:  stun.NewShortTermIntegrity("pass"),
				_nonce:     stun.NewNonce("nonce"),
				log:        logging.NewDefaultLoggerFactory().NewLogger("test"),
			},
			bindingMgr: newBindingManager(),
		}

		payload := []byte("Hello")
		n, err := conn.WriteTo(payload, addr)
		assert.NoError(t, err)

		// The SendIndication STUN message (captured in writeTo above) should be larger
		// than the payload due to headers/attributes.
		assert.Greater(t, len(writtenData), len(payload),
			"STUN message should be larger than payload")

		// WriteTo MUST return the real payload length, not the STUN message length.
		assert.Equal(t, len(payload), n,
			"WriteTo should return payload length (%d), not STUN message length (%d)",
			len(payload), len(writtenData))
	})

	t.Run("ChannelBind transaction failure retains channel number", func(t *testing.T) {
		addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9999}
		serverAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 3478}

		pm := newPermissionMap()
		assert.True(t, pm.insert(addr, &permission{st: permStatePermitted}))

		bm := newBindingManager()
		bound := bm.create(addr)
		originalCh := bound.number

		client := &mockClient{
			performTransaction: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
				return TransactionResult{}, errFake
			},
			writeTo: func(data []byte, _ net.Addr) (int, error) {
				return len(data), nil
			},
		}

		conn := UDPConn{
			allocation: allocation{
				client:     client,
				serverAddr: serverAddr,
				permMap:    pm,
				username:   stun.NewUsername("user"),
				realm:      stun.NewRealm("realm"),
				integrity:  stun.NewShortTermIntegrity("pass"),
				_nonce:     stun.NewNonce("nonce"),
				log:        logging.NewDefaultLoggerFactory().NewLogger("test"),
			},
			bindingMgr: bm,
		}

		// A failed bind attempt should not remove the binding, so a subsequent WriteTo
		// should not allocate a different channel number for the same peer.
		err := conn.bind(bound)
		assert.ErrorIs(t, err, errFake)

		_, _ = conn.WriteTo([]byte("hi"), addr)

		b2, ok := bm.findByAddr(addr)
		assert.True(t, ok)
		assert.Equal(t, originalCh, b2.number)
	})
}

func TestCreatePermissions(t *testing.T) {
	t.Run("CreatePermissions success", func(t *testing.T) {
		called := false
		client := &mockClient{
			performTransaction: func(msg *stun.Message, addr net.Addr, _ bool) (TransactionResult, error) {
				called = true
				// Simulate a successful response
				res := stun.New()
				res.Type = stun.NewType(stun.MethodCreatePermission, stun.ClassSuccessResponse)

				return TransactionResult{Msg: res}, nil
			},
		}
		a := &allocation{
			client:     client,
			serverAddr: &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 3478},
			username:   stun.NewUsername("user"),
			realm:      stun.NewRealm("realm"),
			integrity:  stun.NewShortTermIntegrity("pass"),
			_nonce:     stun.NewNonce("nonce"),
		}
		addr := &net.UDPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 12345}
		err := a.CreatePermissions(addr)
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("CreatePermissions error", func(t *testing.T) {
		client := &mockClient{
			performTransaction: func(msg *stun.Message, addr net.Addr, _ bool) (TransactionResult, error) {
				res := stun.New()
				res.Type = stun.NewType(stun.MethodCreatePermission, stun.ClassErrorResponse)
				code := stun.ErrorCodeAttribute{
					Code:   stun.CodeForbidden,
					Reason: []byte("Forbidden"),
				}
				_ = code.AddTo(res)

				return TransactionResult{Msg: res}, nil
			},
		}
		a := &allocation{
			client:     client,
			serverAddr: &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 3478},
			username:   stun.NewUsername("user"),
			realm:      stun.NewRealm("realm"),
			integrity:  stun.NewShortTermIntegrity("pass"),
			_nonce:     stun.NewNonce("nonce"),
		}
		addr := &net.UDPAddr{IP: net.IPv4(5, 6, 7, 8), Port: 12345}
		err := a.CreatePermissions(addr)
		var turnErr *stun.TurnError
		assert.Error(t, err)
		assert.True(t, errors.As(err, &turnErr), "should return a TurnError")
		assert.Equal(t, stun.CodeForbidden, turnErr.ErrorCodeAttr.Code)
	})
}
