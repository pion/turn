// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"

	"github.com/pion/stun/v2"
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
