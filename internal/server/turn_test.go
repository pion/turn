// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package server

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/allocation"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestAllocationLifeTime(t *testing.T) {
	t.Run("Parsing", func(t *testing.T) {
		lifetime := proto.Lifetime{
			Duration: 5 * time.Second,
		}

		m := &stun.Message{}
		lifetimeDuration := allocationLifeTime(m)
		assert.Equal(t, proto.DefaultLifetime, lifetimeDuration,
			"Allocation lifetime should be default time duration")
		assert.NoError(t, lifetime.AddTo(m))

		lifetimeDuration = allocationLifeTime(m)
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

		lifetimeDuration := allocationLifeTime(m2)
		assert.Equal(t, proto.DefaultLifetime, lifetimeDuration)
	})

	t.Run("DeletionZeroLifetime", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, conn.Close())
		}()

		logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

		allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
			AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
				con, listenErr := net.ListenPacket(network, "0.0.0.0:0")
				if err != nil {
					return nil, nil, listenErr
				}

				return con, con.LocalAddr(), nil
			},
			AllocateConn: func(string, int) (net.Conn, net.Addr, error) {
				return nil, nil, nil
			},
			LeveledLogger: logger,
		})
		assert.NoError(t, err)

		nonceHash, err := NewNonceHash()
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

		_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, 0, time.Hour, "user")
		assert.NoError(t, err)

		assert.NotNil(t, req.AllocationManager.GetAllocation(fiveTuple))

		m := &stun.Message{}
		assert.NoError(t, (proto.Lifetime{}).AddTo(m))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(staticKey)).AddTo(m))

		assert.NoError(t, handleRefreshRequest(req, m))
		assert.Nil(t, req.AllocationManager.GetAllocation(fiveTuple))
	})

	t.Run("QuotaAllocation", func(t *testing.T) {
		conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, conn.Close())
		}()

		logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

		allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
			AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
				con, listenErr := net.ListenPacket(network, "0.0.0.0:0")
				if err != nil {
					return nil, nil, listenErr
				}

				return con, con.LocalAddr(), nil
			},
			AllocateConn: func(string, int) (net.Conn, net.Addr, error) {
				return nil, nil, nil
			},
			LeveledLogger: logger,
			UserQuota:     1,
		})
		assert.NoError(t, err)

		nonceHash, err := NewNonceHash()
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

		stunMsg := &stun.Message{}
		assert.NoError(t, (proto.Lifetime{Duration: 5 * time.Minute}).AddTo(stunMsg))
		assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(stunMsg))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(stunMsg))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(stunMsg))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(stunMsg))
		assert.NoError(t, (stun.Username(staticKey)).AddTo(stunMsg))

		assert.NoError(t, handleAllocateRequest(req, stunMsg))
		assert.NotNil(t, req.AllocationManager.GetAllocation(fiveTuple))

		reqNew := Request{
			AllocationManager: allocationManager,
			NonceHash:         nonceHash,
			Conn:              conn,
			SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5001},
			Log:               logger,
			AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
				return []byte(staticKey), true
			},
		}

		// new allocation will fail
		assert.Error(t, handleAllocateRequest(reqNew, stunMsg))

		// let's delete the prev. allocation
		req.AllocationManager.DeleteAllocation(fiveTuple)

		// now, the new allocation should go through
		assert.NoError(t, handleAllocateRequest(reqNew, stunMsg))
	})
}
