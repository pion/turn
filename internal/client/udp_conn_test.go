// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestUDPConn(t *testing.T) {
	staleTime := func() time.Time {
		return time.Now().Add(-(bindingRefreshInterval + 1*time.Minute))
	}

	staleNonceMsg := func() *stun.Message {
		return stun.MustBuild(
			stun.NewType(stun.MethodChannelBind, stun.ClassErrorResponse),
			stun.CodeStaleNonce,
			stun.NewNonce("new-nonce-123"),
		)
	}

	makeConn := func(client *mockClient, bm *bindingManager) UDPConn {
		return UDPConn{
			allocation: allocation{
				client: client,
				log:    logging.NewDefaultLoggerFactory().NewLogger("test"),
			},
			bindingMgr: bm,
		}
	}

	t.Run("maybeBind()", func(t *testing.T) {
		makeClient := func(shouldSucceed bool) *mockClient {
			return &mockClient{
				performTransaction: func(msg *stun.Message, addr net.Addr, dontWait bool) (TransactionResult, error) {
					if shouldSucceed {
						return TransactionResult{Msg: new(stun.Message)}, nil
					}

					return TransactionResult{Msg: staleNonceMsg()}, nil
				},
			}
		}

		t.Run("state transitions", func(t *testing.T) {
			bm := newBindingManager()
			bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
			conn := makeConn(makeClient(true), bm)

			t.Run("idle to request", func(t *testing.T) {
				bound.setState(bindingStateIdle)
				conn.maybeBind(bound)
				assert.Equal(t, bindingStateRequest, bound.state(), "should be request")
			})

			t.Run("ready to refresh (past interval)", func(t *testing.T) {
				bound.setState(bindingStateReady)
				bound.setRefreshedAt(staleTime())
				conn.maybeBind(bound)
				assert.Equal(t, bindingStateRefresh, bound.state(), "should be refresh")
			})

			t.Run("ready to ready (not past interval)", func(t *testing.T) {
				bound.setState(bindingStateReady)
				bound.setRefreshedAt(time.Now())
				conn.maybeBind(bound)
				assert.Equal(t, bindingStateReady, bound.state(), "should remain ready")
			})
		})

		t.Run("outcomes", func(t *testing.T) {
			tests := []struct {
				name          string
				initialState  bindingState
				pastInterval  bool
				shouldSucceed bool
				expectState   bindingState
			}{
				{"idle bind success", bindingStateIdle, false, true, bindingStateReady},
				{"idle bind fail", bindingStateIdle, false, false, bindingStateFailed},
				{"refresh success", bindingStateReady, true, true, bindingStateReady},
				{"refresh fail", bindingStateReady, true, false, bindingStateFailed},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					bm := newBindingManager()
					bound := bm.create(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234})
					conn := makeConn(makeClient(tt.shouldSucceed), bm)

					bound.setState(tt.initialState)
					if tt.pastInterval {
						bound.setRefreshedAt(staleTime())
					}

					conn.maybeBind(bound)
					assert.Eventually(t, func() bool {
						return bound.state() == tt.expectState
					}, 5*time.Second, 10*time.Millisecond)
				})
			}
		})
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
				expectNonceChanged:   false,
			},
			{
				name: "ErrorResponse with CodeStaleNonce triggers nonce update",
				transactionFn: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
					return TransactionResult{Msg: staleNonceMsg()}, nil
				},
				expectErr:            errTryAgain,
				expectBindingDeleted: false,
				expectNonceChanged:   true,
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
