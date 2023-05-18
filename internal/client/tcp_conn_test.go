// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/transport/v2"
	"github.com/pion/transport/v2/stdnet"
	"github.com/pion/turn/v2/internal/proto"
	"github.com/stretchr/testify/assert"
)

type dummyTCPConn struct {
	transport.TCPConn
}

func buildMsg(transactionID [stun.TransactionIDSize]byte, msgType stun.MessageType, additional ...stun.Setter) []stun.Setter {
	return append([]stun.Setter{&stun.Message{TransactionID: transactionID}, msgType}, additional...)
}

func (c dummyTCPConn) Write(b []byte) (int, error) {
	return len(b), nil
}

func (c dummyTCPConn) Read(b []byte) (int, error) {
	transactionID := [stun.TransactionIDSize]byte{1, 2, 3}
	messageType := stun.MessageType{Method: stun.MethodConnectionBind, Class: stun.ClassSuccessResponse}
	attrs := buildMsg(transactionID, messageType)
	msg, err := stun.Build(attrs...)
	if err != nil {
		return 0, err
	}

	copy(b, msg.Raw)
	return len(msg.Raw), nil
}

type dummyConnObserver struct {
	net                 transport.Net
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

func (obs *dummyConnObserver) Net() transport.Net {
	if obs.net == nil {
		n, err := stdnet.NewNet()
		if err != nil {
			return nil
		}
		obs.net = n
	}
	return obs.net
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
	t.Run("Connect()", func(t *testing.T) {
		var cid proto.ConnectionID = 5
		obs := &dummyConnObserver{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
				if msg.Type.Class == stun.ClassRequest && msg.Type.Method == stun.MethodConnect {
					msg, err := stun.Build(
						stun.TransactionID,
						stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse),
						cid,
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

		actualCid, err := alloc.Connect(addr)
		assert.NoError(t, err)
		assert.Equal(t, cid, actualCid)

		obs = &dummyConnObserver{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
				if msg.Type.Class == stun.ClassRequest && msg.Type.Method == stun.MethodConnect {
					msg, err = stun.Build(
						stun.TransactionID,
						stun.NewType(stun.MethodConnect, stun.ClassErrorResponse),
						stun.ErrorCodeAttribute{Code: stun.CodeBadRequest},
					)
					assert.NoError(t, err)
					return TransactionResult{Msg: msg}, nil
				}
				return TransactionResult{}, errFake
			},
		}
		alloc = TCPAllocation{
			allocation: allocation{
				client:  obs,
				permMap: pm,
				log:     log,
			},
		}

		_, err = alloc.Connect(addr)
		assert.ErrorContains(t, err, "Connect error response", "error 400")
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

	t.Run("AcceptTCPWithConn()", func(t *testing.T) {
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

		from, err := net.ResolveTCPAddr("tcp", "127.0.0.1:11111")
		var cid proto.ConnectionID = 5
		assert.NoError(t, err)
		alloc.HandleConnectionAttempt(from, cid)

		conn := dummyTCPConn{}
		dataConn, err := alloc.AcceptTCPWithConn(conn)
		assert.Equal(t, cid, dataConn.ConnectionID)
		assert.NoError(t, err)
	})

	t.Run("DialWithConn()", func(t *testing.T) {
		relayedAddr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:13478")
		assert.NoError(t, err)

		var cid proto.ConnectionID = 5
		loggerFactory := logging.NewDefaultLoggerFactory()
		obs := &dummyConnObserver{
			_performTransaction: func(msg *stun.Message, to net.Addr, dontWait bool) (TransactionResult, error) {
				typ := stun.NewType(stun.MethodConnect, stun.ClassSuccessResponse)
				if msg.Type.Method == stun.MethodCreatePermission {
					typ = stun.NewType(stun.MethodCreatePermission, stun.ClassSuccessResponse)
				}

				msg, err = stun.Build(
					stun.TransactionID,
					typ,
					cid,
				)
				assert.NoError(t, err)
				return TransactionResult{Msg: msg}, nil
			},
		}
		alloc := NewTCPAllocation(&AllocationConfig{
			Client:      obs,
			Lifetime:    time.Second,
			Log:         loggerFactory.NewLogger("test"),
			RelayedAddr: relayedAddr,
		})

		conn := dummyTCPConn{}
		dataConn, err := alloc.DialWithConn(conn, "tcp", "127.0.0.1:11111")
		assert.Equal(t, cid, dataConn.ConnectionID)
		assert.NoError(t, err)
	})
}
