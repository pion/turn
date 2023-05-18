// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"

	"github.com/pion/stun"
	"github.com/pion/transport/v2"
	"github.com/stretchr/testify/assert"
)

type dummyClient struct {
	turnServerAddr      net.Addr
	username            stun.Username
	realm               stun.Realm
	_writeTo            func(data []byte, to net.Addr) (int, error)
	_performTransaction func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error)
	_onDeallocated      func(relayedAddr net.Addr)
}

func (obs *dummyClient) TURNServerAddr() net.Addr {
	return obs.turnServerAddr
}

func (obs *dummyClient) Username() stun.Username {
	return obs.username
}

func (obs *dummyClient) Realm() stun.Realm {
	return obs.realm
}

func (obs *dummyClient) Net() transport.Net {
	return nil
}

func (obs *dummyClient) WriteTo(data []byte, to net.Addr) (int, error) {
	if obs._writeTo != nil {
		return obs._writeTo(data, to)
	}
	return 0, nil
}

func (obs *dummyClient) PerformTransaction(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
	if obs._performTransaction != nil {
		return obs._performTransaction(msg, to, dontWait)
	}
	return TransactionResult{}, nil
}

func (obs *dummyClient) OnDeallocated(relayedAddr net.Addr) {
	if obs._onDeallocated != nil {
		obs._onDeallocated(relayedAddr)
	}
}

func TestUDPConn(t *testing.T) {
	t.Run("bind()", func(t *testing.T) {
		client := &dummyClient{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
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
		client := &dummyClient{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
				return TransactionResult{}, errFake
			},
			_writeTo: func(data []byte, to net.Addr) (int, error) {
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
