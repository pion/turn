// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package allocation

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/ipnet"
	"github.com/pion/turn/v4/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestAllocation(t *testing.T) {
	tt := []struct {
		name string
		f    func(*testing.T)
	}{
		{"GetPermission", subTestGetPermission},
		{"AddPermission", subTestAddPermission},
		{"RemovePermission", subTestRemovePermission},
		{"AddChannelBind", subTestAddChannelBind},
		{"GetChannelByNumber", subTestGetChannelByNumber},
		{"GetChannelByAddr", subTestGetChannelByAddr},
		{"RemoveChannelBind", subTestRemoveChannelBind},
		{"Refresh", subTestAllocationRefresh},
		{"Close", subTestAllocationClose},
		{"packetHandler", subTestPacketHandler},
		{"ResponseCache", subTestResponseCache},
	}

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t)
		})
	}
}

func subTestGetPermission(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	addr2, err := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	addr3, err := net.ResolveUDPAddr("udp", "127.0.0.2:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	perms := &Permission{
		Addr: addr,
	}
	perms2 := &Permission{
		Addr: addr2,
	}
	perms3 := &Permission{
		Addr: addr3,
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

func subTestAddPermission(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	p := &Permission{
		Addr: addr,
	}

	alloc.AddPermission(p)
	assert.Equal(t, alloc, p.allocation, "Permission's allocation should be the adder.")

	foundPermission := alloc.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission)
}

func subTestRemovePermission(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	p := &Permission{
		Addr: addr,
	}

	alloc.AddPermission(p)

	foundPermission := alloc.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission, "Got permission is not same as the the added.")

	alloc.RemovePermission(p.Addr)

	foundPermission = alloc.GetPermission(p.Addr)
	assert.Nil(t, foundPermission, "Got permission should be nil after removed.")
}

func subTestAddChannelBind(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	err = alloc.AddChannelBind(c, proto.DefaultLifetime)
	assert.Nil(t, err, "should succeed")
	assert.Equal(t, alloc, c.allocation, "allocation should be the caller.")

	c2 := NewChannelBind(proto.MinChannelNumber+1, addr, nil)
	err = alloc.AddChannelBind(c2, proto.DefaultLifetime)
	assert.NotNil(t, err, "should failed with conflicted peer address")

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	c3 := NewChannelBind(proto.MinChannelNumber, addr2, nil)
	err = alloc.AddChannelBind(c3, proto.DefaultLifetime)
	assert.NotNil(t, err, "should fail with conflicted number.")
}

func subTestGetChannelByNumber(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime)

	existChannel := alloc.GetChannelByNumber(c.Number)
	assert.Equal(t, c, existChannel)

	notExistChannel := alloc.GetChannelByNumber(proto.MinChannelNumber + 1)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func subTestGetChannelByAddr(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime)

	existChannel := alloc.GetChannelByAddr(c.Peer)
	assert.Equal(t, c, existChannel)

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	notExistChannel := alloc.GetChannelByAddr(addr2)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func subTestRemoveChannelBind(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = alloc.AddChannelBind(c, proto.DefaultLifetime)

	alloc.RemoveChannelBind(c.Number)

	channelByNumber := alloc.GetChannelByNumber(c.Number)
	assert.Nil(t, channelByNumber)

	channelByAddr := alloc.GetChannelByAddr(c.Peer)
	assert.Nil(t, channelByAddr)
}

func subTestAllocationRefresh(t *testing.T) {
	t.Helper()

	alloc := NewAllocation(nil, nil, nil)

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

func subTestAllocationClose(t *testing.T) {
	t.Helper()

	network := "udp"

	l, err := net.ListenPacket(network, "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	alloc := NewAllocation(nil, nil, nil)
	alloc.RelaySocket = l
	// Add mock lifetimeTimer
	alloc.lifetimeTimer = time.AfterFunc(proto.DefaultLifetime, func() {})

	// Add channel
	addr, err := net.ResolveUDPAddr(network, "127.0.0.1:3478")
	if err != nil {
		t.Fatalf("failed to resolve: %s", err)
	}

	c := NewChannelBind(proto.MinChannelNumber, addr, nil)
	_ = alloc.AddChannelBind(c, proto.DefaultLifetime)

	// Add permission
	alloc.AddPermission(NewPermission(addr, nil))

	err = alloc.Close()
	assert.Nil(t, err, "should succeed")
	assert.True(t, isClose(alloc.RelaySocket), "should be closed")
}

func subTestPacketHandler(t *testing.T) {
	t.Helper()

	network := "udp"

	manager, _ := newTestManager()

	// TURN server initialization
	turnSocket, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	// Client listener initialization
	clientListener, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

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
	}, turnSocket, 0, proto.DefaultLifetime)

	assert.Nil(t, err, "should succeed")

	peerListener1, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	peerListener2, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	// Add permission with peer1 address
	alloc.AddPermission(NewPermission(peerListener1.LocalAddr(), manager.log))
	// Add channel with min channel number and peer2 address
	channelBind := NewChannelBind(proto.MinChannelNumber, peerListener2.LocalAddr(), manager.log)
	_ = alloc.AddChannelBind(channelBind, proto.DefaultLifetime)

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
	err = stun.Decode(data, &msg)
	assert.Nil(t, err, "decode data to stun message failed")

	var msgData proto.Data
	err = msgData.GetFrom(&msg)
	assert.Nil(t, err, "get data from stun message failed")
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
	err = channelData.Decode()
	assert.Nil(t, err, fmt.Sprintf("channel data decode with error: %v", err))
	assert.Equal(t, channelBind.Number, channelData.Number, "get channel data's number is invalid")
	assert.Equal(t, targetText2, string(channelData.Data), "get data doesn't equal the target text.")

	// Listeners close
	_ = manager.Close()
	_ = clientListener.Close()
	_ = peerListener1.Close()
	_ = peerListener2.Close()
}

func subTestResponseCache(t *testing.T) {
	t.Helper()

	a := NewAllocation(nil, nil, nil)
	transactionID := [stun.TransactionIDSize]byte{1, 2, 3}
	responseAttrs := []stun.Setter{
		&proto.Lifetime{
			Duration: proto.DefaultLifetime,
		},
	}
	a.SetResponseCache(transactionID, responseAttrs)

	cacheID, cacheAttr := a.GetResponseCache()
	assert.Equal(t, transactionID, cacheID)
	assert.Equal(t, responseAttrs, cacheAttr)
}
