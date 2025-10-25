// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"errors"
	"net"
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestUDPConn(t *testing.T) {
	t.Run("bind()", func(t *testing.T) {
		client := &mockClient{
			performTransaction: func(*stun.Message, net.Addr, bool) (TransactionResult, error) {
				return TransactionResult{}, errFake
			},
		}

		bm := newBindingManager()
		b := bm.create(&net.UDPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 1234,
		})

		conn := UDPConn{
			allocation: allocation{
				client: client,
			},
			bindingMgr: bm,
		}

		err := conn.bind(b)
		assert.Error(t, err, "should fail")
		assert.Equal(t, 0, len(bm.chanMap), "should be 0")
		assert.Equal(t, 0, len(bm.addrMap), "should be 0")
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
