// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/proto"
	"github.com/stretchr/testify/assert"
)

type dummyConnObserver struct {
	turnServerAddr      net.Addr
	username            stun.Username
	realm               stun.Realm
	_writeTo            func(data []byte, to net.Addr) (int, error)
	_performTransaction func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error)
	_onDeallocated      func(relayedAddr net.Addr)
}

func (obs *dummyConnObserver) TURNServerAddr() net.Addr {
	return obs.turnServerAddr
}

func (obs *dummyConnObserver) Username() stun.Username {
	return obs.username
}

func (obs *dummyConnObserver) Realm() stun.Realm {
	return obs.realm
}

func (obs *dummyConnObserver) WriteTo(data []byte, to net.Addr) (int, error) {
	if obs._writeTo != nil {
		return obs._writeTo(data, to)
	}
	return 0, nil
}

func (obs *dummyConnObserver) PerformTransaction(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
	if obs._performTransaction != nil {
		return obs._performTransaction(msg, to, dontWait)
	}
	return TransactionResult{}, nil
}

func (obs *dummyConnObserver) OnDeallocated(relayedAddr net.Addr) {
	if obs._onDeallocated != nil {
		obs._onDeallocated(relayedAddr)
	}
}

func TestTCPConn(t *testing.T) {
	t.Run("connect()", func(t *testing.T) {
		var serverCid proto.ConnectionID = 4567
		obs := &dummyConnObserver{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
				if msg.Type.Class == stun.ClassRequest && msg.Type.Method == stun.MethodConnect {
					msg, err := stun.Build(
						stun.TransactionID,
						stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse),
						serverCid,
					)
					assert.NoError(t, err)
					return TransactionResult{Msg: msg}, nil
				}
				return TransactionResult{}, errFake
			},
		}

		addr := &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 1234,
		}

		pm := newPermissionMap()
		assert.True(t, pm.insert(addr, &permission{
			st: permStatePermitted,
		}))

		loggerFactory := logging.NewDefaultLoggerFactory()
		log := loggerFactory.NewLogger("test")
		alloc := TCPAllocation{
			allocation: allocation{
				client:  obs,
				permMap: pm,
				log:     log,
			},
		}

		cid, err := alloc.Connect(addr)
		assert.Equal(t, serverCid, cid)
		assert.NoError(t, err)
	})

	t.Run("SetDeadline()", func(t *testing.T) {
		relayedAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:13478")
		assert.NoError(t, err)

		loggerFactory := logging.NewDefaultLoggerFactory()
		obs := &dummyConnObserver{}
		alloc := NewTCPAllocation(&AllocationConfig{
			Client:      obs,
			Lifetime:    time.Second,
			Log:         loggerFactory.NewLogger("test"),
			RelayedAddr: relayedAddr,
		})

		err = alloc.SetDeadline(time.Now())
		assert.NoError(t, err)

		cid, err := alloc.AcceptTCPWithConn(nil)
		assert.Nil(t, cid)
		assert.Contains(t, err.Error(), "i/o timeout")
	})
}
