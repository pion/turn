// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package allocation

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/ipnet"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestAllocation_MarshalUnmarshalBinary(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		fiveTuple := &FiveTuple{
			SrcAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
			DstAddr: &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678},
		}

		turnSocket, err := (&net.ListenConfig{}).ListenPacket(context.Background(), "udp4", "127.0.0.1:0")
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, turnSocket.Close())
		}()

		loggerFactory := logging.NewDefaultLoggerFactory()
		log := loggerFactory.NewLogger("test")

		alloc := NewAllocation(turnSocket, fiveTuple, EventHandler{}, log)
		alloc.Protocol = UDP
		alloc.username = "user"
		alloc.realm = "realm"
		alloc.expiresAt = time.Now().Add(time.Minute).Round(time.Second)

		relayAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:1000")
		assert.NoError(t, err)
		alloc.RelayAddr = relayAddr

		permAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:2000")
		assert.NoError(t, err)
		alloc.AddPermission(NewPermission(permAddr, log, DefaultPermissionTimeout))

		chanAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3000")
		assert.NoError(t, err)
		err = alloc.AddChannelBind(NewChannelBind(proto.MinChannelNumber, chanAddr, log),
			proto.DefaultLifetime,
			DefaultPermissionTimeout,
		)
		assert.NoError(t, err)

		data, err := alloc.MarshalBinary()
		assert.NoError(t, err)

		newAlloc := NewAllocation(nil, &FiveTuple{}, EventHandler{}, log)
		// Initialize lifetimeTimer before unmarshaling, as Refresh expects it.
		// Use a dummy long-running timer so it doesn't fire during the test.
		newAlloc.lifetimeTimer = time.AfterFunc(time.Hour, func() {})
		err = newAlloc.UnmarshalBinary(data)
		assert.NoError(t, err)
		assert.Equal(t, alloc.fiveTuple.String(), newAlloc.fiveTuple.String())
		assert.Equal(t, alloc.Protocol, newAlloc.Protocol)
		assert.Equal(t, alloc.username, newAlloc.username)
		assert.Equal(t, alloc.realm, newAlloc.realm)
		assert.True(t, alloc.expiresAt.Equal(newAlloc.expiresAt))
		assert.Equal(t, alloc.RelayAddr.String(), newAlloc.RelayAddr.String())
		assert.Equal(t, len(alloc.permissions), len(newAlloc.permissions))
		assert.Equal(t, len(alloc.channelBindings), len(newAlloc.channelBindings))
	})
}

func TestGetPermission(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	addr2, err := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	assert.NoError(t, err)

	addr3, err := net.ResolveUDPAddr("udp", "127.0.0.2:3478")
	assert.NoError(t, err)

	perms := &Permission{
		Addr:    addr,
		timeout: DefaultPermissionTimeout,
	}
	perms2 := &Permission{
		Addr:    addr2,
		timeout: DefaultPermissionTimeout,
	}
	perms3 := &Permission{
		Addr:    addr3,
		timeout: DefaultPermissionTimeout,
	}

	alloc.AddPermission(perms)
	alloc.AddPermission(perms2)
	alloc.AddPermission(perms3)

	foundP1 := alloc.GetPermission(addr)
	assert.Equal(t, perms, foundP1, "Should keep the first one.")

	foundP2 := alloc.GetPermission(addr2)
	assert.Equal(t, perms, foundP2, "Second one should be ignored.")

	foundP3 := alloc.GetPermission(addr3)
	assert.Equal(t, perms3, foundP3, "Permission with another IP should be found")
}

func TestAddPermission(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	p := &Permission{
		Addr:    addr,
		timeout: DefaultPermissionTimeout,
	}

	alloc.AddPermission(p)
	assert.Equal(t, alloc, p.allocation, "Permission's allocation should be the adder.")

	foundPermission := alloc.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission)
}

func TestRemovePermission(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	p := &Permission{
		Addr:    addr,
		timeout: DefaultPermissionTimeout,
	}

	alloc.AddPermission(p)

	foundPermission := alloc.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission, "Got permission is not same as the the added.")

	alloc.RemovePermission(p.Addr)

	foundPermission = alloc.GetPermission(p.Addr)
	assert.Nil(t, foundPermission, "Got permission should be nil after removed.")
}

func TestAddChannelBind(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	err = alloc.AddChannelBind(c, proto.DefaultLifetime, DefaultPermissionTimeout)
	assert.Nil(t, err, "should succeed")
	assert.Equal(t, alloc, c.allocation, "allocation should be the caller.")

	c2 := NewChannelBind(proto.MinChannelNumber+1, addr, nil)
	err = alloc.AddChannelBind(c2, proto.DefaultLifetime, DefaultPermissionTimeout)
	assert.NotNil(t, err, "should failed with conflicted peer address")

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	c3 := NewChannelBind(proto.MinChannelNumber, addr2, nil)
	err = alloc.AddChannelBind(c3, proto.DefaultLifetime, DefaultPermissionTimeout)
	assert.NotNil(t, err, "should fail with conflicted number.")
}

func TestGetChannelByNumber(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime, DefaultPermissionTimeout)

	existChannel := alloc.GetChannelByNumber(c.Number)
	assert.Equal(t, c, existChannel)

	notExistChannel := alloc.GetChannelByNumber(proto.MinChannelNumber + 1)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func TestGetChannelByAddr(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime, DefaultPermissionTimeout)

	existChannel := alloc.GetChannelByAddr(c.Peer)
	assert.Equal(t, c, existChannel)

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	notExistChannel := alloc.GetChannelByAddr(addr2)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func TestRemoveChannelBind(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	assert.NoError(t, err)

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime, DefaultPermissionTimeout)

	alloc.RemoveChannelBind(c.Number)

	channelByNumber := alloc.GetChannelByNumber(c.Number)
	assert.Nil(t, channelByNumber)

	channelByAddr := alloc.GetChannelByAddr(c.Peer)
	assert.Nil(t, channelByAddr)
}

func TestAllocationRefresh(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	alloc.lifetimeTimer = time.AfterFunc(proto.DefaultLifetime, func() {
		wg.Done()
	})
	alloc.Refresh(0)
	wg.Wait()

	// LifetimeTimer has expired
	assert.False(t, alloc.lifetimeTimer.Stop())
}

func TestAllocationClose(t *testing.T) {
	network := "udp"

	l, err := (&net.ListenConfig{}).ListenPacket(context.Background(), network, "0.0.0.0:0")
	assert.NoError(t, err)

	alloc := NewAllocation(nil, nil, EventHandler{}, nil)
	alloc.RelaySocket = l
	// Add mock lifetimeTimer
	alloc.lifetimeTimer = time.AfterFunc(proto.DefaultLifetime, func() {})

	// Add channel
	addr, err := net.ResolveUDPAddr(network, "127.0.0.1:3478")
	assert.NoError(t, err)

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)
	_ = alloc.AddChannelBind(c, proto.DefaultLifetime, DefaultPermissionTimeout)

	// Add permission
	alloc.AddPermission(NewPermission(addr, nil, DefaultPermissionTimeout))

	assert.Nil(t, alloc.Close(), "should succeed")
	assert.True(t, isClose(alloc.RelaySocket), "should be closed")
}

func TestPacketHandler(t *testing.T) {
	network := "udp"

	manager, _ := newTestManager()

	// TURN server initialization
	turnSocket, err := (&net.ListenConfig{}).ListenPacket(context.Background(), network, "127.0.0.1:0")
	assert.NoError(t, err)

	// Client listener initialization
	clientListener, err := (&net.ListenConfig{}).ListenPacket(context.Background(), network, "127.0.0.1:0")
	assert.NoError(t, err)

	dataCh := make(chan []byte)
	// Client listener read data
	go func() {
		buffer := make([]byte, rtpMTU)
		for {
			n, _, err2 := clientListener.ReadFrom(buffer)
			if err2 != nil {
				return
			}

			dataCh <- buffer[:n]
		}
	}()

	alloc, err := manager.CreateAllocation(&FiveTuple{
		SrcAddr: clientListener.LocalAddr(),
		DstAddr: turnSocket.LocalAddr(),
	}, turnSocket, 0, proto.DefaultLifetime, "", "")

	assert.NoError(t, err, "should succeed")

	peerListener1, err := (&net.ListenConfig{}).ListenPacket(context.Background(), network, "127.0.0.1:0")
	assert.NoError(t, err)

	peerListener2, err := (&net.ListenConfig{}).ListenPacket(context.Background(), network, "127.0.0.1:0")
	assert.NoError(t, err)

	// Add permission with peer1 address
	alloc.AddPermission(NewPermission(peerListener1.LocalAddr(), manager.log, DefaultPermissionTimeout))
	// Add channel with min channel number and peer2 address
	channelBind := NewChannelBind(proto.MinChannelNumber, peerListener2.LocalAddr(), manager.log)
	_ = alloc.AddChannelBind(channelBind, proto.DefaultLifetime, DefaultPermissionTimeout)

	_, port, _ := ipnet.AddrIPPort(alloc.RelaySocket.LocalAddr())
	relayAddrWithHostStr := fmt.Sprintf("127.0.0.1:%d", port)
	relayAddrWithHost, _ := net.ResolveUDPAddr(network, relayAddrWithHostStr)

	// Test for permission and data message
	targetText := "permission"
	_, _ = peerListener1.WriteTo([]byte(targetText), relayAddrWithHost)
	data := <-dataCh

	// Resolve stun data message
	assert.True(t, stun.IsMessage(data), "should be stun message")

	var msg stun.Message
	assert.NoError(t, stun.Decode(data, &msg), "decode data to stun message failed")

	var msgData proto.Data
	assert.NoError(t, msgData.GetFrom(&msg), "get data from stun message failed")
	assert.Equal(t, targetText, string(msgData), "get message doesn't equal the target text")

	// Test for channel bind and channel data
	targetText2 := "channel bind"
	_, _ = peerListener2.WriteTo([]byte(targetText2), relayAddrWithHost)
	data = <-dataCh

	// Resolve channel data
	assert.True(t, proto.IsChannelData(data), "should be channel data")

	channelData := proto.ChannelData{
		Raw: data,
	}
	assert.NoError(t, channelData.Decode(), fmt.Sprintf("channel data decode with error: %v", err))
	assert.Equal(t, channelBind.Number, channelData.Number, "get channel data's number is invalid")
	assert.Equal(t, targetText2, string(channelData.Data), "get data doesn't equal the target text.")

	// Listeners close
	_ = manager.Close()
	_ = clientListener.Close()
	_ = peerListener1.Close()
	_ = peerListener2.Close()
}

func TestResponseCache(t *testing.T) {
	alloc := NewAllocation(nil, nil, EventHandler{}, nil)
	transactionID := [stun.TransactionIDSize]byte{1, 2, 3}
	responseAttrs := []stun.Setter{
		&proto.Lifetime{
			Duration: proto.DefaultLifetime,
		},
	}
	alloc.SetResponseCache(transactionID, responseAttrs)

	cacheID, cacheAttr := alloc.GetResponseCache()
	assert.Equal(t, transactionID, cacheID)
	assert.Equal(t, responseAttrs, cacheAttr)
}
