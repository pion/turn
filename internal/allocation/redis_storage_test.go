// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func newTestRedisClient() (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	_, err := client.Ping(context.Background()).Result()

	return client, err
}

func testStorage(t *testing.T, storage Storage) {
	t.Helper()

	fiveTuple := randomFiveTuple()
	log := logging.NewDefaultLoggerFactory().NewLogger("test")

	// Test AddAllocation and GetAllocation
	alloc := NewAllocation(nil, fiveTuple, EventHandler{}, log)
	storage.AddAllocation(alloc)

	retrievedAlloc, ok := storage.GetAllocation(fiveTuple.Fingerprint())
	assert.True(t, ok, "Failed to get allocation")
	assert.NotNil(t, retrievedAlloc, "Retrieved allocation is nil")
	assert.Equal(t, alloc.fiveTuple.Fingerprint(), retrievedAlloc.fiveTuple.Fingerprint(), "Fingerprints do not match")

	// Test GetAllocations
	allocs := storage.GetAllocations()
	assert.Len(t, allocs, 1, "Expected 1 allocation")

	// Test DeleteAllocation
	storage.DeleteAllocation(fiveTuple.Fingerprint())
	_, ok = storage.GetAllocation(fiveTuple.Fingerprint())
	assert.False(t, ok, "Allocation should have been deleted")

	// Test Close
	assert.NoError(t, storage.Close(), "Failed to close storage")
}

func TestMemoryStorage(t *testing.T) {
	s := NewMemoryStorage()
	testStorage(t, s)
}

func TestRedisStorage(t *testing.T) {
	client, err := newTestRedisClient()
	if err != nil {
		t.Skip("Redis server not available")
	}
	s := NewRedisStorage(client)
	testStorage(t, s)
}

func TestManager_LoadSave(t *testing.T) {
	manager, err := newTestManager()
	assert.NoError(t, err)

	turnSocket, err := (&net.ListenConfig{}).ListenPacket(context.Background(), "udp4", "0.0.0.0:0")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, turnSocket.Close())
	}()

	// Create some allocations
	allocs := []*Allocation{}
	for i := 0; i < 3; i++ {
		a, errCreate := manager.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Minute, "user", "realm")
		assert.NoError(t, errCreate)
		allocs = append(allocs, a)
	}

	// Save allocations
	manager.SaveAllocations()

	// Create a new manager with the same storage
	m2, err := NewManager(ManagerConfig{
		LeveledLogger:      manager.log,
		AllocatePacketConn: manager.allocatePacketConn,
		AllocateConn:       manager.allocateConn,
		PermissionHandler:  manager.permissionHandler,
		EventHandler:       manager.EventHandler,
		Storage:            manager.storage,
	})
	assert.NoError(t, err)

	// Load allocations
	m2.LoadAllocations(turnSocket)

	// Check if allocations are loaded
	assert.Equal(t, len(allocs), m2.AllocationCount())
	for _, a := range allocs {
		loadedAlloc := m2.GetAllocation(a.fiveTuple)
		assert.NotNil(t, loadedAlloc)
		assert.Equal(t, a.fiveTuple.Fingerprint(), loadedAlloc.fiveTuple.Fingerprint())
	}
}
