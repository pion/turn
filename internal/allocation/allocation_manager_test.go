// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package allocation

import (
	"context"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2/internal/proto"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

func TestManager(t *testing.T) {
	tt := []struct {
		name string
		f    func(*testing.T, net.PacketConn)
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
			f(t, turnSocket)
		})
	}
}

// Test invalid Allocation creations
func subTestCreateInvalidAllocation(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	if a, err := m.CreateAllocation(nil, turnSocket, 0, proto.DefaultLifetime, proto.ProtoUDP); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil FiveTuple")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), nil, 0, proto.DefaultLifetime, proto.ProtoUDP); a != nil || err == nil {
		t.Errorf("Illegally created allocation with nil turnSocket")
	}
	if a, err := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, 0, proto.ProtoUDP); a != nil || err == nil {
		t.Errorf("Illegally created allocation with 0 lifetime")
	}
}

// Test valid Allocation creations
func subTestCreateAllocation(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, proto.ProtoUDP); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a := m.GetAllocation(fiveTuple); a == nil {
		t.Errorf("Failed to get allocation right after creation")
	}
}

// Test that two allocations can't be created with the same FiveTuple
func subTestCreateAllocationDuplicateFiveTuple(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, proto.ProtoUDP); a == nil || err != nil {
		t.Errorf("Failed to create allocation %v %v", a, err)
	}

	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, proto.ProtoUDP); a != nil || err == nil {
		t.Errorf("Was able to create allocation with same FiveTuple twice")
	}
}

func subTestDeleteAllocation(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	if a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, proto.DefaultLifetime, proto.ProtoUDP); a == nil || err != nil {
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
func subTestAllocationTimeout(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	allocations := make([]*Allocation, 5)
	lifetime := time.Second

	for index := range allocations {
		fiveTuple := randomFiveTuple()

		a, err := m.CreateAllocation(fiveTuple, turnSocket, 0, lifetime, proto.ProtoUDP)
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

// test for binding connection
func subTestBindConnection(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	a, _ := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Second, proto.ProtoTCP)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}
	cid, err := m.Connect(a, listener.Addr())
	if err != nil {
		t.Errorf("Connect error: %v", err)
	}

	conn := m.BindConnection(cid)
	assert.NotNil(t, conn)

	listener.Close()
	if err := m.Close(); err != nil {
		t.Errorf("Manager close with error: %v", err)
	}
}

// test for manager close
func subTestManagerClose(t *testing.T, turnSocket net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	allocations := make([]*Allocation, 2)

	a1, _ := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Second, proto.ProtoUDP)
	allocations[0] = a1
	a2, _ := m.CreateAllocation(randomFiveTuple(), turnSocket, 0, time.Minute, proto.ProtoUDP)
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
	/* #nosec */
	return &FiveTuple{
		SrcAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
		DstAddr: &net.UDPAddr{IP: nil, Port: rand.Int()},
	}
}

func newTestManager() (*Manager, error) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	config := ManagerConfig{
		LeveledLogger: loggerFactory.NewLogger("test"),
		AllocatePacketConn: func(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
			conn, err := net.ListenPacket("udp4", "0.0.0.0:0")
			if err != nil {
				return nil, nil, err
			}

			return conn, conn.LocalAddr(), nil
		},
		AllocateListener: func(network string, requestedPort int) (net.Listener, net.Addr, error) {
			config := &net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
				var err error
				c.Control(func(fd uintptr) {
					err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR|unix.SO_REUSEPORT, 1)
				})
				return err
			}}
			listener, err := config.Listen(context.Background(), network, "127.0.0.1:"+strconv.Itoa(requestedPort))
			if err != nil {
				return nil, nil, err
			}

			return listener, listener.Addr(), nil
		},
	}
	return NewManager(config)
}

func isClose(conn io.Closer) bool {
	closeErr := conn.Close()
	return closeErr != nil && strings.Contains(closeErr.Error(), "use of closed network connection")
}

func subTestGetRandomEvenPort(t *testing.T, _ net.PacketConn) {
	m, err := newTestManager()
	assert.NoError(t, err)

	port, err := m.GetRandomEvenPort()
	assert.NoError(t, err)
	assert.True(t, port > 0)
	assert.True(t, port%2 == 0)
}
