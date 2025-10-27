// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestUDPConn(t *testing.T) {
	makeConn := func(client *mockClient, bm *bindingManager) UDPConn {
		return UDPConn{
			allocation: allocation{
				client: client,
				log:    logging.NewDefaultLoggerFactory().NewLogger("test"),
			},
			bindingMgr: bm,
		}
	}

	staleNonceMsg := func() *stun.Message {
		return stun.MustBuild(
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			stun.CodeStaleNonce,
			stun.NewNonce("new-nonce-123"),
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
					bound.setRefreshedAt(time.Now().Add(-(bindingRefreshInterval + 1*time.Minute)))
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
			expectBindingDeleted bool
			expectNonceChanged   bool
		}{
			{
				name: "PerformTransaction returns error",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					return TransactionResult{}, errFake
				},
				expectErr:            errFake,
				expectBindingDeleted: true,
			},
			{
				name: "ErrorResponse with CodeStaleNonce triggers nonce update",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					return TransactionResult{Msg: staleNonceMsg()}, nil
				},
				expectErr:          errTryAgain,
				expectNonceChanged: true,
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

				if tt.expectBindingDeleted {
					assert.Empty(t, bm.chanMap)
					assert.Empty(t, bm.addrMap)
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
