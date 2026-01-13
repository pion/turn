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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/transport/v4/reuseport"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestNewManagerValidation(t *testing.T) {
	loggerFactory := logging.NewDefaultLoggerFactory()

	cfg := ManagerConfig{}

	manager, err := NewManager(cfg)
	assert.Nil(t, manager)
	assert.ErrorIs(t, err, errAllocatePacketConnMustBeSet)

	cfg.AllocatePacketConn = func(string, int) (net.PacketConn, net.Addr, error) { return nil, nil, nil }
	manager, err = NewManager(cfg)
	assert.Nil(t, manager)
	assert.ErrorIs(t, err, errAllocateListenerMustBeSet)

	cfg.AllocateListener = func(string, int) (net.Listener, net.Addr, error) { return nil, nil, nil }
	manager, err = NewManager(cfg)
	assert.Nil(t, manager)
	assert.ErrorIs(t, err, errAllocateConnMustBeSet)

	cfg.AllocateConn = func(network string, laddr, raddr net.Addr) (net.Conn, error) { return nil, nil } //nolint:nilnil
	manager, err = NewManager(cfg)
	assert.Nil(t, manager)
	assert.ErrorIs(t, err, errLeveledLoggerMustBeSet)

	cfg.LeveledLogger = loggerFactory.NewLogger("test")
	manager, err = NewManager(cfg)
	assert.NotNil(t, manager)
	assert.NoError(t, err)
	assert.NoError(t, manager.Close())
}

// Test invalid Allocation creations.
func TestCreateInvalidAllocation(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	alloc, err := manager.CreateAllocation(nil, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.Nil(t, alloc, "Illegally created allocation with nil FiveTuple")
	assert.Error(t, err, "Illegally created allocation with nil FiveTuple")

	alloc, err = manager.CreateAllocation(randomFiveTuple(), nil, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.Nil(t, alloc, "Illegally created allocation with nil turnSocket")
	assert.Error(t, err, "Illegally created allocation with nil turnSocket")

	alloc, err = manager.CreateAllocation(randomFiveTuple(), turnSocket, proto.ProtoUDP,
		0, 0, "", "", proto.RequestedFamilyIPv4)
	assert.Nil(t, alloc, "Illegally created allocation with 0 lifetime")
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
	alloc, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NotNil(t, alloc, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	alloc = manager.GetAllocation(fiveTuple)
	assert.NotNil(t, alloc, "Failed to get allocation right after creation")

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
	alloc, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NotNil(t, alloc, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	alloc, err = manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.Nil(t, alloc, "Was able to create allocation with same FiveTuple twice")
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
	alloc, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NotNil(t, alloc, "Failed to create allocation")
	assert.NoError(t, err, "Failed to create allocation")

	alloc = manager.GetAllocation(fiveTuple)
	assert.NotNil(t, alloc, "Failed to get allocation right after creation")

	manager.DeleteAllocation(fiveTuple)
	alloc = manager.GetAllocation(fiveTuple)
	assert.Nilf(t, alloc, "Failed to delete allocation %v", fiveTuple)

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

		alloc, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
			0, lifetime, "", "", proto.RequestedFamilyIPv4)
		assert.NoErrorf(t, err, "Failed to create allocation with %v", fiveTuple)

		allocations[index] = alloc
	}

	// Make sure all allocations timeout
	time.Sleep(lifetime + time.Second)
	for _, alloc := range allocations {
		assert.True(t, isClose(alloc.relayPacketConn), "Allocation relay socket should be closed if lifetime timeout")
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

	a1, _ := manager.CreateAllocation(randomFiveTuple(), turnSocket, proto.ProtoUDP,
		0, time.Second, "", "", proto.RequestedFamilyIPv4)
	allocations[0] = a1
	a2, _ := manager.CreateAllocation(randomFiveTuple(), turnSocket, proto.ProtoUDP,
		0, time.Minute, "", "", proto.RequestedFamilyIPv4)
	allocations[1] = a2

	// Make a1 timeout
	time.Sleep(2 * time.Second)
	assert.NoError(t, manager.Close())

	for _, alloc := range allocations {
		assert.True(t, isClose(alloc.relayPacketConn), "Manager's allocations should be closed")
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
		AllocateListener: func(string, int) (net.Listener, net.Addr, error) { return nil, nil, nil },
		AllocateConn: func(network string, laddr, raddr net.Addr) (net.Conn, error) {
			dialer := net.Dialer{
				LocalAddr: laddr,
				Control:   reuseport.Control,
			}

			return dialer.Dial(network, raddr.String())
		},
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

func TestCreateTCPConnection(t *testing.T) {
	lns := make([]net.Listener, 3)
	mu := sync.Mutex{} // make the race detector happy
	acceptedConns := make([]net.Conn, 3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	for i := 0; i < 3; i++ {
		lns[i], err = net.Listen("tcp", "127.0.0.1:0") // nolint: noctx
		assert.NoError(t, err)

		go func(j int) {
			conn, connErr := lns[j].Accept()
			assert.NoError(t, connErr)

			mu.Lock()
			acceptedConns[j] = conn
			mu.Unlock()

			if j == 2 {
				cancel()
			}
		}(i)
	}

	manager, err := newTestManager()
	assert.NoError(t, err)

	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoTCP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)
	allocation.RelayAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: rand.Intn(60999-32768+1) + 32768} //nolint:gosec

	for i := 0; i < 3; i++ {
		addr, ok := lns[i].Addr().(*net.TCPAddr)
		assert.True(t, ok)
		peer := proto.PeerAddress{IP: addr.IP, Port: addr.Port}

		connectionID, err := manager.CreateTCPConnection(allocation, peer)
		assert.NoError(t, err)
		assert.NotZero(t, connectionID)

		conn := manager.GetTCPConnection("", connectionID)
		assert.NotNil(t, conn)

		laddr, ok := conn.LocalAddr().(*net.TCPAddr)
		assert.True(t, ok)
		relayAddr, ok := allocation.RelayAddr.(*net.TCPAddr)
		assert.True(t, ok)
		assert.Equal(t, laddr.IP.String(), relayAddr.IP.String())
		assert.Equal(t, laddr.IP.String(), relayAddr.IP.String())

		assert.NoError(t, conn.Close())
	}

	<-ctx.Done()

	assert.NoError(t, turnSocket.Close())

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < 3; i++ {
		assert.NoError(t, acceptedConns[i].Close())
		assert.NoError(t, lns[i].Close())
	}
}

func TestCreateTCPConnectionDuplicateTCPConn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // nolint: noctx
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var acceptedConn net.Conn
	var acceptErr error
	go func() {
		acceptedConn, acceptErr = ln.Accept()
		cancel()
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	assert.True(t, ok)

	peer := proto.PeerAddress{IP: addr.IP, Port: addr.Port}

	manager, err := newTestManager()
	assert.NoError(t, err)

	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoTCP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	allocation.RelayAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: rand.Intn(60999-32768+1) + 32768} //nolint:gosec
	connectionID, err := manager.CreateTCPConnection(allocation, peer)
	assert.NoError(t, err)
	assert.NotZero(t, connectionID)

	_, err = manager.CreateTCPConnection(allocation, peer)
	assert.ErrorIs(t, err, ErrDupeTCPConnection)

	<-ctx.Done()
	assert.NoError(t, acceptErr)
	assert.NoError(t, acceptedConn.Close())
	assert.NoError(t, ln.Close())
	assert.NoError(t, turnSocket.Close())
}

func TestCreateTCPConnectionInvalidPeerAddress(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoTCP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)
	allocation.RelayAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: rand.Intn(60999-32768+1) + 32768} //nolint:gosec

	_, err = manager.CreateTCPConnection(allocation, proto.PeerAddress{IP: nil, Port: 1234})
	assert.ErrorIs(t, err, errInvalidPeerAddress)

	_, err = manager.CreateTCPConnection(allocation, proto.PeerAddress{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	assert.ErrorIs(t, err, errInvalidPeerAddress)

	assert.NoError(t, turnSocket.Close())
}

func TestCreateTCPConnectionInvalid(t *testing.T) {
	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	manager, err := newTestManager()
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoTCP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)
	allocation.RelayAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: rand.Intn(60999-32768+1) + 32768} //nolint:gosec

	peerAddress := proto.PeerAddress{IP: net.ParseIP("127.0.0.1"), Port: 5000}

	connectionID, err := manager.CreateTCPConnection(allocation, peerAddress)
	assert.ErrorIs(t, err, ErrTCPConnectionTimeoutOrFailure)
	assert.Zero(t, connectionID)

	assert.NoError(t, turnSocket.Close())
}

func TestCreateTCPConnectionTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // nolint: noctx
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var acceptedConn net.Conn
	var acceptErr error
	go func() {
		acceptedConn, acceptErr = ln.Accept()
		cancel()
	}()

	addr, ok := ln.Addr().(*net.TCPAddr)
	assert.True(t, ok)

	peer := proto.PeerAddress{IP: addr.IP, Port: addr.Port}

	manager, err := newTestManager()
	assert.NoError(t, err)
	manager.tcpConnectionBindTimeout = time.Millisecond

	turnSocket, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)

	fiveTuple := randomFiveTuple()
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoTCP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)
	allocation.RelayAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: rand.Intn(60999-32768+1) + 32768} //nolint:gosec

	connectionID, err := manager.CreateTCPConnection(allocation, peer)
	assert.NoError(t, err)
	assert.NotZero(t, connectionID)

	time.Sleep(time.Millisecond * 100)
	assert.Nil(t, manager.GetTCPConnection("", connectionID))

	<-ctx.Done()
	assert.NoError(t, acceptErr)
	assert.NoError(t, acceptedConn.Close())
	assert.NoError(t, ln.Close())
	assert.NoError(t, turnSocket.Close())
}

func TestCreateAllocationIPv6(t *testing.T) {
	manager, err := newTestManager()
	assert.NoError(t, err)

	// Create a UDP6 socket for TURN
	turnSocket, err := net.ListenPacket("udp6", "[::]:0") // nolint: noctx
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, turnSocket.Close())
	}()

	fiveTuple := randomFiveTuple()
	// Change the source address to IPv6
	fiveTuple.SrcAddr = &net.UDPAddr{IP: net.ParseIP("::1"), Port: 5000}

	// Create an IPv6 allocation
	allocation, err := manager.CreateAllocation(fiveTuple, turnSocket, proto.ProtoUDP,
		0, proto.DefaultLifetime, "", "", proto.RequestedFamilyIPv6)
	assert.NoError(t, err)
	assert.NotNil(t, allocation)
	assert.Equal(t, proto.RequestedFamilyIPv6, allocation.AddressFamily())

	// Verify allocation is retrievable
	foundAllocation := manager.GetAllocation(fiveTuple)
	assert.NotNil(t, foundAllocation)
	assert.Equal(t, proto.RequestedFamilyIPv6, foundAllocation.AddressFamily())

	// Delete allocation
	manager.DeleteAllocation(fiveTuple)
	assert.Nil(t, manager.GetAllocation(fiveTuple))
}
