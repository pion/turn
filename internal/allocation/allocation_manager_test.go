// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package allocation

import (
	"io"
	"math/rand"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

// Test invalid Allocation creations.
func TestCreateInvalidAllocation(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	a, err := manager.CreateAllocation(nil, turnSocket, 0, proto.DefaultLifetime, "", "")
	assert.Nil(t, a, "Illegally created allocation with nil FiveTuple")
	assert.Error(t, err, "Illegally created allocation with nil FiveTuple")

	a, err = manager.CreateAllocation(randomFiveTuple(), nil, 0, proto.DefaultLifetime, "", "")
	assert.Nil(t, a, "Illegally created allocation with nil turnSocket")
	assert.Error(t, err, "Illegally created allocation with nil turnSocket")

	a, err = manager.CreateAllocation(randomFiveTuple(), turnSocket, 0, 0, "", "")
	assert.Nil(t, a, "Illegally created allocation with 0 lifetime")
	assert.Error(t, err, "Illegally created allocation with 0 lifetime")

	assert.NoError(t, manager.Close())
	assert.NoError(t, turnSocket.Close())
}

// Test valid Allocation creations.
func TestCreateAllocation(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	a, err := manager.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, "", "")
	assert.NotNil(t, a, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	a = manager.GetAllocation(fiveTuple)
	assert.NotNil(t, a, "Failed to get allocation right after creation")

	assert.NoError(t, manager.Close())
	assert.NoError(t, turnSocket.Close())
}

// Test that two allocations can't be created with the same FiveTuple.
func TestCreateAllocationDuplicateFiveTuple(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	a, err := manager.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, "", "")
	assert.NotNil(t, a, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	a, err = manager.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, "", "")
	assert.Nil(t, a, "Was able to create allocation with same FiveTuple twice")
	assert.Error(t, err, "Was able to create allocation with same FiveTuple twice")

	assert.NoError(t, manager.Close())
	assert.NoError(t, turnSocket.Close())
}

func TestDeleteAllocation(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	a, err := manager.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, "", "")
	assert.NotNil(t, a, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	a = manager.GetAllocation(fiveTuple)
	assert.NotNil(t, a, "Failed to get allocation right after creation")

	manager.DeleteAllocation(fiveTuple)
	a = manager.GetAllocation(fiveTuple)
	assert.Nilf(t, a, "Failed to delete allocation %v", fiveTuple)

	assert.NoError(t, manager.Close())
	assert.NoError(t, turnSocket.Close())
}

// Test that allocation should be closed if timeout.
func TestAllocationTimeout(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	allocations := make([]*Allocation, 5)
	lifetime := time.Second

	for index := range allocations {
		fiveTuple := randomFiveTuple()

		a, err := manager.CreateAllocation(fiveTuple, turnSocket, 0, lifetime, "", "")
		assert.NoErrorf(t, err, "Failed to create allocation with %v", fiveTuple)

		allocations[index] = a
	}

	// Make sure all allocations timeout
	time.Sleep(lifetime + time.Second)
	for _, alloc := range allocations {
		assert.True(t, isClose(alloc.RelaySocket), "Allocation relay socket should be closed if lifetime timeout")
	}

	assert.NoError(t, manager.Close())
	assert.NoError(t, turnSocket.Close())
}

// Test for manager close.
func TestManagerClose(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	allocations := make([]*Allocation, 2)

	a1, _ := manager.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Second, "", "")
	allocations[0] = a1
	a2, _ := manager.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Minute, "", "")
	allocations[1] = a2

	// Make a1 timeout
	time.Sleep(2 * time.Second)
	assert.NoError(t, manager.Close())

	for _, alloc := range allocations {
		assert.True(t, isClose(alloc.RelaySocket), "Manager's allocations should be closed")
	}

	assert.NoError(t, turnSocket.Close())
}

func randomFiveTuple() *FiveTuple {
	// nolint
	return &FiveTuple{
		SrcAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
		DstAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
	}
}

func newTestManager() (*Manager, error) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	config := ManagerConfig{
		LeveledLogger: loggerFactory.NewLogger("test"),
		AllocatePacketConn: func(string, int) (net.PacketConn, net.Addr, error) {
			conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, err
			}

			return conn, conn.LocalAddr(), nil
		},
		AllocateConn: func(string, int) (net.Conn, net.Addr, error) { return nil, nil, nil },
	}

	return NewManager(config)
}

func isClose(conn io.Closer) bool {
	closeErr := conn.Close()

	return closeErr != nil && strings.Contains(closeErr.Error(), "use of closed network connection")
}

func TestGetRandomEvenPort(t *testing.T) {
	manager, err := newTestManager()
	assert.NoError(t, err)

	port, err := manager.GetRandomEvenPort()
	assert.NoError(t, err)
	assert.True(t, port > 0)
	assert.True(t, port%2 == 0)

	assert.NoError(t, manager.Close())
}
