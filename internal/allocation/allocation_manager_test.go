// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package allocation

import (
	"errors"
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

var (
	errUnexpectedTestRealm    = errors.New("unexpected test realm")
	errUnexpectedTestUsername = errors.New("unexpected user name")
)

func TestManager(t *testing.T) {
	tt := []struct {
		name string
		f    func(*testing.T, net.PacketConn, string, string)
	}{
		{"CreateInvalidAllocation", subTestCreateInvalidAllocation},
		{"CreateAllocation", subTestCreateAllocation},
		{"CreateAllocationDuplicateFiveTuple", subTestCreateAllocationDuplicateFiveTuple},
		{"DeleteAllocation", subTestDeleteAllocation},
		{"AllocationTimeout", subTestAllocationTimeout},
		{"Close", subTestManagerClose},
		{"GetRandomEvenPort", subTestGetRandomEvenPort},
	}

	network := "udp4"
	turnSocket, err := net.ListenPacket(network, "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t, turnSocket, "test_realm_1", "test_user_1")
		})
	}
}

// Test invalid Allocation creations
func subTestCreateInvalidAllocation(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	metadata := Metadata{Realm: realm, Username: username}
	if a, err := m.CreateAllocation(nil, turnSocket, 0, proto.DefaultLifetime, metadata); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil FiveTuple")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), nil, 0, proto.DefaultLifetime, metadata); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil turnSocket")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, 0, metadata); a != nil || err == nil {
		t.Errorf("Illegally created allocation with 0 lifetime")
	}
}

// Test valid Allocation creations
func subTestCreateAllocation(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	metadata := Metadata{Realm: realm, Username: username}
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, metadata); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a := m.GetAllocation(fiveTuple); a == nil {
		t.Errorf("Failed to get allocation right after creation")
	}
}

// Test that two allocations can't be created with the same FiveTuple
func subTestCreateAllocationDuplicateFiveTuple(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	metadata := Metadata{Realm: realm, Username: username}
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, metadata); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, metadata); a != nil || err == nil {
		t.Errorf("Was able to create allocation with same FiveTuple twice")
	}
}

func subTestDeleteAllocation(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	metadata := Metadata{Realm: realm, Username: username}
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, metadata); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a := m.GetAllocation(fiveTuple); a == nil {
		t.Errorf("Failed to get allocation right after creation")
	}

	m.DeleteAllocation(fiveTuple)
	if a := m.GetAllocation(fiveTuple); a != nil {
		t.Errorf("Get allocation with %v should be nil after delete", fiveTuple)
	}
}

// Test that allocation should be closed if timeout
func subTestAllocationTimeout(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	allocations := make([]*Allocation, 5)
	lifetime := time.Second
	metadata := Metadata{Realm: realm, Username: username}
	for index := range allocations {
		fiveTuple := randomFiveTuple()

		a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, lifetime, metadata)
		if err != nil {
			t.Errorf("Failed to create allocation with %v", fiveTuple)
		}

		allocations[index] = a
	}

	// Make sure all allocations timeout
	time.Sleep(lifetime + time.Second)
	for _, alloc := range allocations {
		if !isClose(alloc.RelaySocket) {
			t.Error("Allocation relay socket should be closed if lifetime timeout")
		}
	}
}

// Test for manager close
func subTestManagerClose(t *testing.T, turnSocket net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	allocations := make([]*Allocation, 2)
	metadata := Metadata{Realm: realm, Username: username}
	a1, _ := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Second, metadata)
	allocations[0] = a1
	a2, _ := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Minute, metadata)
	allocations[1] = a2

	// Make a1 timeout
	time.Sleep(2 * time.Second)

	if err := m.Close(); err != nil {
		t.Errorf("Manager close with error: %v", err)
	}

	for _, alloc := range allocations {
		if !isClose(alloc.RelaySocket) {
			t.Error("Manager's allocations should be closed")
		}
	}
}

func randomFiveTuple() *FiveTuple {
	// nolint
	return &FiveTuple{
		SrcAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
		DstAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
	}
}

func newTestManager(expectedRealm, expectedUsername string) (*Manager, error) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	config := ManagerConfig{
		LeveledLogger: loggerFactory.NewLogger("test"),
		AllocatePacketConn: func(_ string, _ int, metadata Metadata) (net.PacketConn, net.Addr, error) {
			if metadata.Realm != expectedRealm {
				return nil, nil, errUnexpectedTestRealm
			}
			if metadata.Username != expectedUsername {
				return nil, nil, errUnexpectedTestUsername
			}
			conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
			if err != nil {
				return nil, nil, err
			}

			return conn, conn.LocalAddr(), nil
		},
		AllocateConn: func(string, int, Metadata) (net.Conn, net.Addr, error) { return nil, nil, nil },
	}

	return NewManager(config)
}

func isClose(conn io.Closer) bool {
	closeErr := conn.Close()
	return closeErr != nil && strings.Contains(closeErr.Error(), "use of closed network connection")
}

func subTestGetRandomEvenPort(t *testing.T, _ net.PacketConn, realm, username string) {
	m, err := newTestManager(realm, username)
	assert.NoError(t, err)

	metadata := Metadata{Realm: realm, Username: username}
	port, err := m.GetRandomEvenPort(metadata)
	assert.NoError(t, err)
	assert.True(t, port > 0)
	assert.True(t, port%2 == 0)
}
