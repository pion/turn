// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/transport/v3/test"
	"github.com/pion/turn/v4/internal/allocation"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestAllocationLifeTime(t *testing.T) {
	report := test.CheckRoutines(t)
	defer report()

	t.Run("Parsing", func(t *testing.T) {
		lifetime := proto.Lifetime{
			Duration: 5 * time.Second,
		}

		m := &stun.Message{}
		lifetimeDuration := allocationLifeTime(Request{AllocationLifetime: proto.DefaultLifetime}, m)
		assert.Equal(t, proto.DefaultLifetime, lifetimeDuration,
			"Allocation lifetime should be default time duration")
		assert.NoError(t, lifetime.AddTo(m))

		lifetimeDuration = allocationLifeTime(Request{AllocationLifetime: proto.DefaultLifetime}, m)
		assert.Equal(t, lifetime.Duration, lifetimeDuration,
			"Allocation lifetime should be equal to the one set in the message")
	})

	// If lifetime is bigger than maximumLifetime
	t.Run("Overflow", func(t *testing.T) {
		lifetime := proto.Lifetime{
			Duration: maximumAllocationLifetime * 2,
		}

		m2 := &stun.Message{}
		_ = lifetime.AddTo(m2)

		lifetimeDuration := allocationLifeTime(Request{AllocationLifetime: proto.DefaultLifetime}, m2)
		assert.Equal(t, proto.DefaultLifetime, lifetimeDuration)
	})

	t.Run("DeletionZeroLifetime", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
		assert.NoError(t, err)

		logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

		allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
			AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
				con, listenErr := net.ListenPacket(network, "0.0.0.0:0") // nolint: noctx
				if err != nil {
					return nil, nil, listenErr
				}

				return con, con.LocalAddr(), nil
			},
			AllocateListener: func(string, int) (net.Listener, net.Addr, error) {
				return nil, nil, nil
			},
			LeveledLogger: logger,
		})
		assert.NoError(t, err)

		nonceHash, err := NewShortNonceHash(0)
		assert.NoError(t, err)
		staticKey, err := nonceHash.Generate()
		assert.NoError(t, err)

		req := Request{
			AllocationManager: allocationManager,
			NonceHash:         nonceHash,
			Conn:              conn,
			SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
			Log:               logger,
			AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
				return []byte(staticKey), true
			},
		}

		fiveTuple := &allocation.FiveTuple{SrcAddr: req.SrcAddr, DstAddr: req.Conn.LocalAddr(), Protocol: allocation.UDP}

		_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP, 0, time.Hour, "test", "")
		assert.NoError(t, err)

		assert.NotNil(t, req.AllocationManager.GetAllocation(fiveTuple))

		m := &stun.Message{}
		assert.NoError(t, (proto.Lifetime{}).AddTo(m))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username("test")).AddTo(m))

		assert.NoError(t, handleRefreshRequest(req, m))
		assert.Nil(t, req.AllocationManager.GetAllocation(fiveTuple))

		assert.NoError(t, conn.Close())
	})
}

func TestRequestedTransport(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(string, int) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager: allocationManager,
		NonceHash:         nonceHash,
		Conn:              conn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:               logger,
		AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
			return []byte(staticKey), true
		},
	}

	m := &stun.Message{}
	assert.NoError(t, (proto.Lifetime{}).AddTo(m))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Username(staticKey)).AddTo(m))
	assert.ErrorIs(t, handleAllocateRequest(req, m), stun.ErrAttributeNotFound)

	assert.NoError(t, (proto.RequestedTransport{Protocol: 1}).AddTo(m))
	assert.ErrorIs(t, handleAllocateRequest(req, m), errUnsupportedTransportProtocol)

	assert.NoError(t, conn.Close())
}

func TestConnectRequest(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(string, int) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager: allocationManager,
		NonceHash:         nonceHash,
		Conn:              conn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:               logger,
		AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
			return []byte(staticKey), true
		},
	}

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0") // nolint: noctx
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		tcpConn, tcpErr := tcpListener.Accept()
		assert.NoError(t, tcpErr)
		assert.NoError(t, tcpConn.Close())
		cancel()
	}()

	fiveTuple := &allocation.FiveTuple{SrcAddr: req.SrcAddr, DstAddr: req.Conn.LocalAddr(), Protocol: allocation.UDP}

	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP, 0, time.Hour, "test", "")
	assert.NoError(t, err)

	stunMsg := &stun.Message{}
	assert.NoError(t, (proto.Lifetime{}).AddTo(stunMsg))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Username("test")).AddTo(stunMsg))
	assert.ErrorIs(t, handleConnectRequest(req, stunMsg), stun.ErrAttributeNotFound)

	assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("127.0.0.1"), Port: 5000}).AddTo(stunMsg))
	assert.ErrorIs(t, handleConnectRequest(req, stunMsg), allocation.ErrTCPConnectionTimeoutOrFailure)

	tcpAddr, ok := tcpListener.Addr().(*net.TCPAddr)
	assert.True(t, ok)

	stunMsg = &stun.Message{}
	assert.NoError(t, (proto.Lifetime{}).AddTo(stunMsg))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Username("test")).AddTo(stunMsg))
	assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("127.0.0.1"), Port: tcpAddr.Port}).AddTo(stunMsg))
	assert.NoError(t, handleConnectRequest(req, stunMsg))
	assert.ErrorIs(t, handleConnectRequest(req, stunMsg), allocation.ErrDupeTCPConnection)

	<-ctx.Done()
	assert.NoError(t, conn.Close())
	assert.NoError(t, tcpListener.Close())
}

func TestConnectionBindRequest(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(string, int) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager: allocationManager,
		NonceHash:         nonceHash,
		Conn:              conn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:               logger,
		AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
			return []byte(staticKey), true
		},
	}

	tcpListener, err := net.Listen("tcp", "127.0.0.1:0") // nolint: noctx
	assert.NoError(t, err)

	go func() {
		tcpConn, tcpErr := tcpListener.Accept()
		assert.NoError(t, tcpConn.Close())
		assert.NoError(t, tcpErr)
	}()

	fiveTuple := &allocation.FiveTuple{SrcAddr: req.SrcAddr, DstAddr: req.Conn.LocalAddr(), Protocol: allocation.UDP}

	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP, 0, time.Hour, "", "")
	assert.NoError(t, err)

	m := &stun.Message{}
	assert.NoError(t, handleConnectionBindRequest(req, m))

	assert.NoError(t, (proto.Lifetime{}).AddTo(m))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Username(staticKey)).AddTo(m))
	assert.ErrorIs(t, handleConnectionBindRequest(req, m), stun.ErrAttributeNotFound)

	assert.NoError(t, (proto.ConnectionID(5)).AddTo(m))
	assert.NoError(t, handleConnectionBindRequest(req, m))
}
