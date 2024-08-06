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
	"github.com/pion/stun/v2"
	"github.com/pion/turn/v3/internal/allocation"
	"github.com/pion/turn/v3/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestAllocationLifeTime(t *testing.T) {
	t.Run("Parsing", func(t *testing.T) {
		lifetime := proto.Lifetime{
			Duration: 5 * time.Second,
		}

		m := &stun.Message{}
		lifetimeDuration := allocationLifeTime(m)

		if lifetimeDuration != proto.DefaultLifetime {
			t.Errorf("Allocation lifetime should be default time duration")
		}

		assert.NoError(t, lifetime.AddTo(m))

		lifetimeDuration = allocationLifeTime(m)
		if lifetimeDuration != lifetime.Duration {
			t.Errorf("Expect lifetimeDuration is %s, but %s", lifetime.Duration, lifetimeDuration)
		}
	})

	// If lifetime is bigger than maximumLifetime
	t.Run("Overflow", func(t *testing.T) {
		lifetime := proto.Lifetime{
			Duration: maximumAllocationLifetime * 2,
		}

		m2 := &stun.Message{}
		_ = lifetime.AddTo(m2)

		lifetimeDuration := allocationLifeTime(m2)
		if lifetimeDuration != proto.DefaultLifetime {
			t.Errorf("Expect lifetimeDuration is %s, but %s", proto.DefaultLifetime, lifetimeDuration)
		}
	})

	t.Run("DeletionZeroLifetime", func(t *testing.T) {
		l, err := net.ListenPacket("udp4", "0.0.0.0:0")
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, l.Close())
		}()

		logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

		allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
			AllocatePacketConn: func(network string, _ int) (net.PacketConn, net.Addr, error) {
				conn, listenErr := net.ListenPacket(network, "0.0.0.0:0")
				if err != nil {
					return nil, nil, listenErr
				}

				return conn, conn.LocalAddr(), nil
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

		authSuccessCallbackTimes := 0

		r := Request{
			AllocationManager: allocationManager,
			NonceHash:         nonceHash,
			Conn:              l,
			SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
			Log:               logger,
			AuthHandler: func(string, string, net.Addr) (key []byte, ok bool) {
				return []byte(staticKey), true
			},

			AuthSuccess: func(username string, realm string, srcAddr net.Addr) {
				authSuccessCallbackTimes++
			},
		}

		fiveTuple := &allocation.FiveTuple{SrcAddr: r.SrcAddr, DstAddr: r.Conn.LocalAddr(), Protocol: allocation.UDP}

		_, err = r.AllocationManager.CreateAllocation(fiveTuple, r.Conn, 0, time.Hour)
		assert.NoError(t, err)
		assert.NotNil(t, r.AllocationManager.GetAllocation(fiveTuple))

		m := &stun.Message{}
		assert.NoError(t, (proto.Lifetime{}).AddTo(m))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(staticKey)).AddTo(m))

		assert.NoError(t, handleCreatePermissionRequest(r, m))
		assert.Equal(t, 1, authSuccessCallbackTimes)

		assert.NoError(t, handleRefreshRequest(r, m))
		assert.Equal(t, 2, authSuccessCallbackTimes)

		assert.Nil(t, r.AllocationManager.GetAllocation(fiveTuple))
	})
}
