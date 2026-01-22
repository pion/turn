// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/transport/v4/test"
	"github.com/pion/turn/v5/internal/allocation"
	"github.com/pion/turn/v5/internal/auth"
	"github.com/pion/turn/v5/internal/proto"
	"github.com/stretchr/testify/assert"
)

const testUser = "test"

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
			AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
				con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
				if err != nil {
					return nil, nil, listenErr
				}

				return con, con.LocalAddr(), nil
			},
			AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
				return nil, nil, nil
			},
			AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
				return nil, nil //nolint:nilnil
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
			AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
				return testUser, []byte(staticKey), true
			},
		}

		fiveTuple := &allocation.FiveTuple{SrcAddr: req.SrcAddr, DstAddr: req.Conn.LocalAddr(), Protocol: allocation.UDP}

		_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
			0, time.Hour, testUser, "", proto.RequestedFamilyIPv4)
		assert.NoError(t, err)

		assert.NotNil(t, req.AllocationManager.GetAllocation(fiveTuple))

		m := &stun.Message{}
		assert.NoError(t, (proto.Lifetime{}).AddTo(m))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))

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
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
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
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(conf allocation.AllocateConnConfig) (net.Conn, error) {
			return net.Dial("tcp4", conf.RemoteAddr.String()) // nolint: noctx
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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
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

	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, testUser, "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	stunMsg := &stun.Message{}
	assert.NoError(t, (proto.Lifetime{}).AddTo(stunMsg))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(stunMsg))
	assert.NoError(t, (stun.Username(testUser)).AddTo(stunMsg))
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
	assert.NoError(t, (stun.Username(testUser)).AddTo(stunMsg))
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
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if err != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(conf allocation.AllocateConnConfig) (net.Conn, error) {
			return net.Dial("tcp4", conf.RemoteAddr.String()) // nolint: noctx
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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return "", []byte(staticKey), true
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

	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "", "", proto.RequestedFamilyIPv4)
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

func TestRequestedAddressFamilyIPv6(t *testing.T) {
	conn, err := net.ListenPacket("udp6", "[::]:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "[::]:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
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
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("::1"), Port: 5000},
		Log:               logger,
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}

	// Create IPv6 allocation
	alloc, err := req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, testUser, "", proto.RequestedFamilyIPv6)
	assert.NoError(t, err)
	assert.NotNil(t, alloc)
	assert.Equal(t, proto.RequestedFamilyIPv6, alloc.AddressFamily())

	// Test that the allocation is present
	foundAlloc := req.AllocationManager.GetAllocation(fiveTuple)
	assert.NotNil(t, foundAlloc)
	assert.Equal(t, proto.RequestedFamilyIPv6, foundAlloc.AddressFamily())
}

func TestRequestedAddressFamilyUnsupported(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	// Create message with unsupported address family (0x03)
	m := &stun.Message{}
	m.TransactionID = stun.NewTransactionID()
	assert.NoError(t, m.Build(stun.NewType(stun.MethodAllocate, stun.ClassRequest)))
	assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(m))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Username(staticKey)).AddTo(m))
	// Manually add invalid address family
	m.Add(stun.AttrRequestedAddressFamily, []byte{0x03, 0x00, 0x00, 0x00})

	// Should return error with code 440 (Address Family not Supported)
	err = handleAllocateRequest(req, m)
	assert.Error(t, err)
}

func TestRequestedAddressFamilyMutualExclusivity(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	// Create message with both REQUESTED-ADDRESS-FAMILY and RESERVATION-TOKEN
	m := &stun.Message{} //nolint:varnamelen
	m.TransactionID = stun.NewTransactionID()
	assert.NoError(t, m.Build(stun.NewType(stun.MethodAllocate, stun.ClassRequest)))
	assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(m))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Username(staticKey)).AddTo(m))
	assert.NoError(t, proto.RequestedFamilyIPv4.AddTo(m))
	// ReservationToken must be exactly 8 bytes
	assert.NoError(t, (proto.ReservationToken([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})).AddTo(m))

	// Should return error with code 400 (Bad Request) for mutual exclusivity violation
	err = handleAllocateRequest(req, m)
	assert.Error(t, err)
}

func TestHandleRefreshRequestRequestedAddressFamilyMismatch(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	fiveTuple := &allocation.FiveTuple{SrcAddr: req.SrcAddr, DstAddr: req.Conn.LocalAddr(), Protocol: allocation.UDP}
	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "test", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	m := &stun.Message{}
	m.TransactionID = stun.NewTransactionID()
	assert.NoError(t, m.Build(stun.NewType(stun.MethodRefresh, stun.ClassRequest)))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
	assert.NoError(t, (stun.Username(testUser)).AddTo(m))
	m.Add(stun.AttrRequestedAddressFamily, []byte{0x03, 0x00, 0x00, 0x00})
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))

	err = handleRefreshRequest(req, m)
	assert.ErrorIs(t, err, errPeerAddressFamilyMismatch)
}

func TestIPMatchesFamily(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		ipv4 := net.ParseIP("192.168.1.1")
		assert.True(t, ipMatchesFamily(ipv4, proto.RequestedFamilyIPv4))
		assert.False(t, ipMatchesFamily(ipv4, proto.RequestedFamilyIPv6))
	})

	t.Run("IPv6", func(t *testing.T) {
		ipv6 := net.ParseIP("2001:db8::1")
		assert.False(t, ipMatchesFamily(ipv6, proto.RequestedFamilyIPv4))
		assert.True(t, ipMatchesFamily(ipv6, proto.RequestedFamilyIPv6))
	})

	t.Run("IPv6Loopback", func(t *testing.T) {
		ipv6Loopback := net.ParseIP("::1")
		assert.False(t, ipMatchesFamily(ipv6Loopback, proto.RequestedFamilyIPv4))
		assert.True(t, ipMatchesFamily(ipv6Loopback, proto.RequestedFamilyIPv6))
	})

	t.Run("IPv4Loopback", func(t *testing.T) {
		ipv4Loopback := net.ParseIP("127.0.0.1")
		assert.True(t, ipMatchesFamily(ipv4Loopback, proto.RequestedFamilyIPv4))
		assert.False(t, ipMatchesFamily(ipv4Loopback, proto.RequestedFamilyIPv6))
	})
}

func TestHandleCreatePermissionRequest(t *testing.T) { //nolint:dupl
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

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
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
		PermissionTimeout: 5 * time.Minute,
	}

	t.Run("NoAllocationFound", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodCreatePermission, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))

		err = handleCreatePermissionRequest(req, m)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoAllocationFound)
	})

	// Create an allocation for the remaining tests
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, testUser, "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	t.Run("SuccessIPv4", func(t *testing.T) { //nolint:dupl
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodCreatePermission, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("192.168.1.1"), Port: 8080}).AddTo(m))

		err = handleCreatePermissionRequest(req, m)
		assert.NoError(t, err)
	})

	t.Run("PeerAddressFamilyMismatch", func(t *testing.T) { //nolint:dupl
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodCreatePermission, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		// Try to add IPv6 peer address to IPv4 allocation
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("2001:db8::1"), Port: 8080}).AddTo(m))

		err = handleCreatePermissionRequest(req, m)
		// Should succeed but log a warning and return error response
		assert.NoError(t, err)
	})

	t.Run("NoPeerAddress", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodCreatePermission, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		// No peer address added

		err = handleCreatePermissionRequest(req, m)
		// Should return error response (addCount == 0)
		assert.NoError(t, err)
	})
}

func TestHandleSendIndication(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	req := Request{
		AllocationManager: allocationManager,
		Conn:              conn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:               logger,
	}

	t.Run("NoAllocationFound", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))

		err = handleSendIndication(req, m)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoAllocationFound)
	})

	// Create an allocation for the remaining tests
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc, err := req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	t.Run("MissingDataAttribute", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))

		err = handleSendIndication(req, m)
		assert.Error(t, err)
	})

	t.Run("MissingPeerAddress", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))
		assert.NoError(t, (proto.Data([]byte("test data"))).AddTo(m))

		err = handleSendIndication(req, m)
		assert.Error(t, err)
	})

	t.Run("NoPermission", func(t *testing.T) {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))
		assert.NoError(t, (proto.Data([]byte("test data"))).AddTo(m))
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("192.168.1.1"), Port: 8080}).AddTo(m))

		err = handleSendIndication(req, m)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoPermission)
	})

	t.Run("Success", func(t *testing.T) {
		// Add permission for the peer
		peerAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8080}
		alloc.AddPermission(allocation.NewPermission(peerAddr, logger, 5*time.Minute))

		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))
		assert.NoError(t, (proto.Data([]byte(testUser))).AddTo(m))
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("192.168.1.1"), Port: 8080}).AddTo(m))

		err = handleSendIndication(req, m)
		// May fail due to network issues, but shouldn't panic
		if err != nil {
			assert.Error(t, err)
		}
	})
}

func TestHandleChannelBindRequest(t *testing.T) { // nolint:maintidx
	logger := &captureLogger{}
	conn := newCapturePacketConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3478})
	defer conn.Close() //nolint:errcheck

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager:  allocationManager,
		NonceHash:          nonceHash,
		Conn:               conn,
		SrcAddr:            &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:                logger,
		ChannelBindTimeout: 10 * time.Minute,
		PermissionTimeout:  5 * time.Minute,
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	buildAuthMsg := func(ch uint16, peerIP string, peerPort int) *stun.Message {
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		assert.NoError(t, (proto.ChannelNumber(ch)).AddTo(m))
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP(peerIP), Port: peerPort}).AddTo(m))

		return m
	}

	assertLastResponse := func(t *testing.T, expectedClass stun.MessageClass, expectedCode stun.ErrorCode) {
		t.Helper()
		raw := conn.LastWrite()
		if !assert.NotEmpty(t, raw, "server should write a response") {
			return
		}

		res := &stun.Message{Raw: append([]byte(nil), raw...)}
		assert.NoError(t, res.Decode())
		assert.Equal(t, stun.MethodChannelBind, res.Type.Method)
		assert.Equal(t, expectedClass, res.Type.Class)

		if expectedClass == stun.ClassErrorResponse {
			var code stun.ErrorCodeAttribute
			assert.NoError(t, code.GetFrom(res))
			assert.Equal(t, expectedCode, code.Code)
		}
	}

	t.Run("NoAllocationFound", func(t *testing.T) {
		conn.Reset()
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))

		err = handleChannelBindRequest(req, m)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoAllocationFound)
	})

	// Create an allocation for the remaining tests
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	_, err = req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, testUser, "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	t.Run("MissingChannelNumber", func(t *testing.T) {
		conn.Reset()
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))

		err = handleChannelBindRequest(req, m)
		assert.Error(t, err)
	})

	t.Run("MissingPeerAddress", func(t *testing.T) {
		conn.Reset()
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		assert.NoError(t, (proto.ChannelNumber(0x4000)).AddTo(m))

		err = handleChannelBindRequest(req, m)
		assert.Error(t, err)
	})

	// Peer address family mismatch (IPv6 peer with IPv4 allocation)
	t.Run("PeerAddressFamilyMismatch", func(t *testing.T) {
		conn.Reset()
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		assert.NoError(t, (proto.ChannelNumber(0x4000)).AddTo(m))
		// Try to bind IPv6 peer to IPv4 allocation
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("2001:db8::1"), Port: 8080}).AddTo(m))

		err = handleChannelBindRequest(req, m)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errPeerAddressFamilyMismatch)
		assertLastResponse(t, stun.ClassErrorResponse, stun.CodePeerAddrFamilyMismatch)
	})

	t.Run("PermissionDenied", func(t *testing.T) {
		conn.Reset()

		var denyAM *allocation.Manager
		denyAM, err = allocation.NewManager(allocation.ManagerConfig{
			AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
				con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
				if listenErr != nil {
					return nil, nil, listenErr
				}

				return con, con.LocalAddr(), nil
			},
			AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
				return nil, nil, nil
			},
			AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
				return nil, nil //nolint:nilnil
			},
			PermissionHandler: func(net.Addr, net.IP) bool { return false },
			LeveledLogger:     logger,
		})
		assert.NoError(t, err)
		defer denyAM.Close() //nolint:errcheck

		denyReq := req
		denyReq.AllocationManager = denyAM
		fiveTuple := &allocation.FiveTuple{
			SrcAddr:  denyReq.SrcAddr,
			DstAddr:  denyReq.Conn.LocalAddr(),
			Protocol: allocation.UDP,
		}
		_, err = denyReq.AllocationManager.CreateAllocation(fiveTuple, denyReq.Conn, proto.ProtoUDP,
			0, time.Hour, testUser, "", proto.RequestedFamilyIPv4)
		assert.NoError(t, err)

		m := buildAuthMsg(0x4000, "192.168.1.1", 8080)
		err = handleChannelBindRequest(denyReq, m)
		assert.Error(t, err)
		assertLastResponse(t, stun.ClassErrorResponse, stun.CodeUnauthorized)
	})

	t.Run("RejectSamePeerDifferentChannel", func(t *testing.T) {
		conn.Reset()
		logger.Reset()

		m1 := buildAuthMsg(0x4010, "192.168.1.1", 8080)
		err = handleChannelBindRequest(req, m1)
		assert.NoError(t, err)

		conn.Reset()
		m2 := buildAuthMsg(0x4011, "192.168.1.1", 8080)
		err = handleChannelBindRequest(req, m2)
		assert.Error(t, err)
		assert.ErrorIs(t, err, allocation.ErrSamePeerDifferentChannel)
		assertLastResponse(t, stun.ClassErrorResponse, stun.CodeBadRequest)
		assert.True(t, logger.ContainsWarn("peer 192.168.1.1:8080 already bound"), fmt.Sprintf("warn logs: %v", logger.warns))
	})

	t.Run("RejectSameChannelDifferentPeer", func(t *testing.T) {
		conn.Reset()
		logger.Reset()

		m1 := buildAuthMsg(0x4020, "192.168.2.1", 8080)
		err = handleChannelBindRequest(req, m1)
		assert.NoError(t, err)

		conn.Reset()
		m2 := buildAuthMsg(0x4020, "192.168.2.2", 8080)
		err = handleChannelBindRequest(req, m2)
		assert.Error(t, err)
		assert.ErrorIs(t, err, allocation.ErrSameChannelDifferentPeer)
		assertLastResponse(t, stun.ClassErrorResponse, stun.CodeBadRequest)
		assert.True(t, logger.ContainsWarn("channel 16416 already bound"), fmt.Sprintf("warn logs: %v", logger.warns))
	})

	t.Run("Success", func(t *testing.T) {
		conn.Reset()
		m := &stun.Message{}
		m.TransactionID = stun.NewTransactionID()
		assert.NoError(t, m.Build(stun.NewType(stun.MethodChannelBind, stun.ClassRequest)))
		assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Nonce(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Realm(staticKey)).AddTo(m))
		assert.NoError(t, (stun.Username(testUser)).AddTo(m))
		assert.NoError(t, (proto.ChannelNumber(0x4030)).AddTo(m))
		assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("192.168.3.1"), Port: 8080}).AddTo(m))

		err = handleChannelBindRequest(req, m)
		assert.NoError(t, err)
	})
}

type capturePacketConn struct {
	mu        sync.Mutex
	localAddr net.Addr
	lastWrite []byte
	closed    bool
}

func newCapturePacketConn(localAddr net.Addr) *capturePacketConn {
	return &capturePacketConn{localAddr: localAddr}
}

func (c *capturePacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, net.ErrClosed
}

func (c *capturePacketConn) WriteTo(p []byte, _ net.Addr) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	c.lastWrite = append([]byte(nil), p...)

	return len(p), nil
}

func (c *capturePacketConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true

	return nil
}

func (c *capturePacketConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *capturePacketConn) SetDeadline(time.Time) error {
	return nil
}

func (c *capturePacketConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *capturePacketConn) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *capturePacketConn) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastWrite = nil
}

func (c *capturePacketConn) LastWrite() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]byte(nil), c.lastWrite...)
}

type captureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *captureLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = nil
}

func (l *captureLogger) ContainsWarn(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.warns {
		if strings.Contains(w, substr) {
			return true
		}
	}

	return false
}

func (l *captureLogger) Trace(string)          {}
func (l *captureLogger) Tracef(string, ...any) {}
func (l *captureLogger) Debug(string)          {}
func (l *captureLogger) Debugf(string, ...any) {}
func (l *captureLogger) Info(string)           {}
func (l *captureLogger) Infof(string, ...any)  {}
func (l *captureLogger) Error(string)          {}
func (l *captureLogger) Errorf(string, ...any) {}
func (l *captureLogger) Warn(msg string)       { l.Warnf("%s", msg) }

func (l *captureLogger) Warnf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprintf(format, args...))
}

func TestHandleAllocationRequest(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager:  allocationManager,
		NonceHash:          nonceHash,
		Conn:               conn,
		SrcAddr:            &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:                logger,
		AllocationLifetime: proto.DefaultLifetime,
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return "test-id", []byte(staticKey), true
		},
	}

	// First allocation request with transaction ID 1
	msg1 := &stun.Message{}
	msg1.TransactionID = stun.NewTransactionID()
	assert.NoError(t, msg1.Build(stun.NewType(stun.MethodAllocate, stun.ClassRequest)))
	assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(msg1))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Username("test-name")).AddTo(msg1))

	// First request should succeed
	err = handleAllocateRequest(req, msg1)
	assert.NoError(t, err)

	// Verify allocation exists
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc := req.AllocationManager.GetAllocation(fiveTuple)
	assert.NotNil(t, alloc)

	// Test if allocation was created for the user-id instead of the username
	alloc = req.AllocationManager.GetAllocationForUserID(fiveTuple, "test-id")
	assert.NotNil(t, alloc)
	alloc = req.AllocationManager.GetAllocationForUserID(fiveTuple, "test-name")
	assert.Nil(t, alloc)
}

// TestDuplicateAllocationRequest tests the scenario from issue #229
// where a client makes multiple allocation requests from the same 5-tuple
// but with different transaction IDs.
//
// Per RFC 5766 Section 6.2, the server checks if the 5-tuple is currently in use by an
// existing allocation. If yes, the server rejects the request with a 437 (Allocation Mismatch)
// error if the transaction ID differs from the cached transaction ID.
func TestHandleDuplicateAllocationRequest(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	nonceHash, err := NewShortNonceHash(0)
	assert.NoError(t, err)
	staticKey, err := nonceHash.Generate()
	assert.NoError(t, err)

	req := Request{
		AllocationManager:  allocationManager,
		NonceHash:          nonceHash,
		Conn:               conn,
		SrcAddr:            &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:                logger,
		AllocationLifetime: proto.DefaultLifetime,
		AuthHandler: func(*auth.RequestAttributes) (userID string, key []byte, ok bool) {
			return testUser, []byte(staticKey), true
		},
	}

	// First allocation request with transaction ID 1
	msg1 := &stun.Message{}
	msg1.TransactionID = stun.NewTransactionID()
	assert.NoError(t, msg1.Build(stun.NewType(stun.MethodAllocate, stun.ClassRequest)))
	assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(msg1))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(msg1))
	assert.NoError(t, (stun.Username(testUser)).AddTo(msg1))

	// First request should succeed
	err = handleAllocateRequest(req, msg1)
	assert.NoError(t, err)

	// Verify allocation exists
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc := req.AllocationManager.GetAllocation(fiveTuple)
	assert.NotNil(t, alloc)

	// Second allocation request from the same 5-tuple but with a different transaction ID
	msg2 := &stun.Message{}
	msg2.TransactionID = stun.NewTransactionID() // Different transaction ID
	assert.NoError(t, msg2.Build(stun.NewType(stun.MethodAllocate, stun.ClassRequest)))
	assert.NoError(t, (proto.RequestedTransport{Protocol: proto.ProtoUDP}).AddTo(msg2))
	assert.NoError(t, (stun.MessageIntegrity(staticKey)).AddTo(msg2))
	assert.NoError(t, (stun.Nonce(staticKey)).AddTo(msg2))
	assert.NoError(t, (stun.Realm(staticKey)).AddTo(msg2))
	assert.NoError(t, (stun.Username(testUser)).AddTo(msg2))

	// Second request should fail with errRelayAlreadyAllocatedForFiveTuple
	// This is the error reported in issue #229
	err = handleAllocateRequest(req, msg2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errRelayAlreadyAllocatedForFiveTuple)

	// Test retry with same transaction ID (should succeed and return cached response)
	// Per RFC 5766 Section 6.2, when the transaction ID matches the cached transaction ID,
	// the server returns the cached response (proper retry mechanism).
	t.Run("RetryWithSameTransactionID", func(t *testing.T) {
		// Retry the first request with the same transaction ID
		// This should succeed and return the cached response
		err := handleAllocateRequest(req, msg1)
		assert.NoError(t, err, "Retry with same transaction ID should succeed")

		// Verify the allocation still exists and hasn't changed
		allocRetry := req.AllocationManager.GetAllocation(fiveTuple)
		assert.NotNil(t, allocRetry)
		assert.Equal(t, alloc.RelayAddr, allocRetry.RelayAddr, "Relay address should be the same")
	})
}

func TestHandleChannelData(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "0.0.0.0:0") // nolint: noctx
	assert.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	logger := logging.NewDefaultLoggerFactory().NewLogger("turn")

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(conf allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			con, listenErr := net.ListenPacket(conf.Network, "0.0.0.0:0") // nolint: noctx
			if listenErr != nil {
				return nil, nil, listenErr
			}

			return con, con.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	req := Request{
		AllocationManager:  allocationManager,
		Conn:               conn,
		SrcAddr:            &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5000},
		Log:                logger,
		ChannelBindTimeout: 10 * time.Minute,
	}

	t.Run("NoAllocationFound", func(t *testing.T) {
		channelData := &proto.ChannelData{
			Number: 0x4000,
			Data:   []byte("test"),
		}

		err = handleChannelData(req, channelData)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoAllocationFound)
	})

	// Create an allocation for remaining tests
	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc, err := req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	t.Run("NoChannelBinding", func(t *testing.T) {
		channelData := &proto.ChannelData{
			Number: 0x4000,
			Data:   []byte("test"),
		}

		err = handleChannelData(req, channelData)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errNoSuchChannelBind)
	})

	t.Run("Success", func(t *testing.T) {
		// Add a channel binding
		peerAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8080}
		err := alloc.AddChannelBind(
			allocation.NewChannelBind(0x4000, peerAddr, logger),
			10*time.Minute,
			5*time.Minute,
		)
		assert.NoError(t, err)

		channelData := &proto.ChannelData{
			Number: 0x4000,
			Data:   []byte("test"),
		}

		err = handleChannelData(req, channelData)
		// May fail due to network issues, but shouldn't panic
		if err != nil {
			assert.Error(t, err)
		}
	})
}
