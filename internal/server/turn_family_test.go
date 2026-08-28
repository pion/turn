// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/turn/v5/internal/allocation"
	"github.com/pion/turn/v5/internal/proto"
	"github.com/stretchr/testify/assert"
)

// blockingRelayConn is a PacketConn used as the relay socket of an
// allocation. ReadFrom blocks until the conn is closed so the relay read
// goroutine stays idle, while WriteTo records relayed payloads.
type blockingRelayConn struct {
	*capturePacketConn
	done      chan struct{}
	closeOnce sync.Once
}

func newBlockingRelayConn(localAddr net.Addr) *blockingRelayConn {
	return &blockingRelayConn{
		capturePacketConn: newCapturePacketConn(localAddr),
		done:              make(chan struct{}),
	}
}

func (c *blockingRelayConn) ReadFrom([]byte) (int, net.Addr, error) {
	<-c.done

	return 0, nil, net.ErrClosed
}

// Close is idempotent: the allocation teardown and the deferred test cleanup
// may both close this conn.
func (c *blockingRelayConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.capturePacketConn.Close()
		close(c.done)
	})

	return err
}

// TestHandleSendIndicationAddressFamilyMismatch verifies that a Send
// indication addressed to a peer of a different address family than the
// allocation is not relayed, matching the RFC 6156 model where the relayed
// transport address family constrains every peer it communicates with.
//
// CreatePermission and ChannelBind already enforce this (443 Peer Address
// Family Mismatch), but handleSendIndication historically relayed the data
// whenever a matching permission existed.
func TestHandleSendIndicationAddressFamilyMismatch(t *testing.T) {
	logger := &captureLogger{}
	turnConn := newCapturePacketConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3478})
	relayConn := newBlockingRelayConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50000})
	defer relayConn.Close() //nolint:errcheck

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			return relayConn, relayConn.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil //nolint:nilnil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	req := Request{
		Conn:              turnConn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 50000},
		AllocationManager: allocationManager,
		Log:               logger,
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc, err := req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	// The STUN handlers refuse to create cross-family permissions (443), but
	// an allocation with a stale or manually injected permission must still
	// not relay cross-family data.
	ipv6Peer := &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 8080}
	alloc.AddPermission(allocation.NewPermission(ipv6Peer, logger, 5*time.Minute))

	m := &stun.Message{}
	m.TransactionID = stun.NewTransactionID()
	assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))
	assert.NoError(t, (proto.Data([]byte("test data"))).AddTo(m))
	assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("2001:db8::1"), Port: 8080}).AddTo(m))

	err = handleSendIndication(req, m)
	assert.ErrorIs(t, err, errPeerAddressFamilyMismatch)
	assert.Empty(t, relayConn.lastWrite, "cross-family data must not be relayed")
}

// TestHandleSendIndicationSameFamilyStillRelays guards the fix against
// over-rejection: same-family Send indications must keep relaying.
func TestHandleSendIndicationSameFamilyStillRelays(t *testing.T) {
	logger := &captureLogger{}
	turnConn := newCapturePacketConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3478})
	relayConn := newBlockingRelayConn(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50001})
	defer relayConn.Close() //nolint:errcheck

	allocationManager, err := allocation.NewManager(allocation.ManagerConfig{
		AllocatePacketConn: func(allocation.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
			return relayConn, relayConn.LocalAddr(), nil
		},
		AllocateListener: func(allocation.AllocateListenerConfig) (net.Listener, net.Addr, error) {
			return nil, nil, nil //nolint:nilnil
		},
		AllocateConn: func(allocation.AllocateConnConfig) (net.Conn, error) {
			return nil, nil //nolint:nilnil
		},
		LeveledLogger: logger,
	})
	assert.NoError(t, err)
	defer allocationManager.Close() //nolint:errcheck

	req := Request{
		Conn:              turnConn,
		SrcAddr:           &net.UDPAddr{IP: net.ParseIP("192.0.2.2"), Port: 50000},
		AllocationManager: allocationManager,
		Log:               logger,
	}

	fiveTuple := &allocation.FiveTuple{
		SrcAddr:  req.SrcAddr,
		DstAddr:  req.Conn.LocalAddr(),
		Protocol: allocation.UDP,
	}
	alloc, err := req.AllocationManager.CreateAllocation(fiveTuple, req.Conn, proto.ProtoUDP,
		0, time.Hour, "", "", proto.RequestedFamilyIPv4)
	assert.NoError(t, err)

	ipv4Peer := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 8080}
	alloc.AddPermission(allocation.NewPermission(ipv4Peer, logger, 5*time.Minute))

	m := &stun.Message{}
	m.TransactionID = stun.NewTransactionID()
	assert.NoError(t, m.Build(stun.NewType(stun.MethodSend, stun.ClassIndication)))
	assert.NoError(t, (proto.Data([]byte("test data"))).AddTo(m))
	assert.NoError(t, (proto.PeerAddress{IP: net.ParseIP("192.168.1.1"), Port: 8080}).AddTo(m))

	assert.NoError(t, handleSendIndication(req, m))
	assert.NotEmpty(t, relayConn.lastWrite, "same-family data must be relayed")
}
