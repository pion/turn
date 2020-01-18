// +build !js

package server

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/allocation"
	"github.com/pion/turn/v2/internal/proto"
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
			AllocatePacketConn: func(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
				conn, listenErr := net.ListenPacket(network, "0.0.0.0:0")
				if err != nil {
					return nil, nil, listenErr
				}

				return conn, conn.LocalAddr(), nil
			},
			AllocateConn: func(network string, requestedPort int) (net.Conn, net.Addr, error) {
				return nil, nil, nil
			},
			LeveledLogger: logger,
		})
		assert.NoError(t, err)

		staticKey := []byte("ABC")
		r := Request{
			AllocationManager: allocationManager,
			Nonces:            &sync.Map{},
			Conn:              l,
			SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
			Log:               logger,
			AuthHandler: func(username string, realm string, srcAddr net.Addr) (key []byte, ok bool) {
				return staticKey, true
			},
		}
		r.Nonces.Store(string(staticKey), time.Now())

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

		assert.NoError(t, handleRefreshRequest(r, m))
		assert.Nil(t, r.AllocationManager.GetAllocation(fiveTuple))
	})
}
