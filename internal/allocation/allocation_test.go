// +build !js

package allocation

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/stun"
	"github.com/pion/turn/v2/internal/ipnet"
	"github.com/pion/turn/v2/internal/proto"
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
	}

	for _, tc := range tt {
		f := tc.f
		t.Run(tc.name, func(t *testing.T) {
			f(t)
		})
	}
}

func subTestGetPermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	addr3, _ := net.ResolveUDPAddr("udp", "127.0.0.2:3478")

	p := &Permission{
		Addr: addr,
	}
	p2 := &Permission{
		Addr: addr2,
	}
	p3 := &Permission{
		Addr: addr3,
	}

	a.AddPermission(p)
	a.AddPermission(p2)
	a.AddPermission(p3)

	foundP1 := a.GetPermission(addr)
	assert.Equal(t, p, foundP1, "Should keep the first one.")

	foundP2 := a.GetPermission(addr2)
	assert.Equal(t, p, foundP2, "Second one should be ignored.")

	foundP3 := a.GetPermission(addr3)
	assert.Equal(t, p3, foundP3, "Permission with another IP should be found")
}

func subTestAddPermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)
	assert.Equal(t, a, p.allocation, "Permission's allocation should be the adder.")

	foundPermission := a.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission)
}

func subTestRemovePermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)

	foundPermission := a.GetPermission(p.Addr)
	assert.Equal(t, p, foundPermission, "Got permission is not same as the the added.")

	a.RemovePermission(p.Addr)

	foundPermission = a.GetPermission(p.Addr)
	assert.Nil(t, foundPermission, "Got permission should be nil after removed.")
}

func subTestAddChannelBind(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	err := a.AddChannelBind(c, proto.DefaultLifetime)
	assert.Nil(t, err, "should succeed")
	assert.Equal(t, a, c.allocation, "allocation should be the caller.")

	c2 := NewChannelBind(proto.MinChannelNumber+1, addr, nil)
	err = a.AddChannelBind(c2, proto.DefaultLifetime)
	assert.NotNil(t, err, "should failed with conflicted peer address")

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	c3 := NewChannelBind(proto.MinChannelNumber, addr2, nil)
	err = a.AddChannelBind(c3, proto.DefaultLifetime)
	assert.NotNil(t, err, "should fail with conflicted number.")
}

func subTestGetChannelByNumber(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, proto.DefaultLifetime)

	existChannel := a.GetChannelByNumber(c.Number)
	assert.Equal(t, c, existChannel)

	notExistChannel := a.GetChannelByNumber(proto.MinChannelNumber + 1)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func subTestGetChannelByAddr(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, proto.DefaultLifetime)

	existChannel := a.GetChannelByAddr(c.Peer)
	assert.Equal(t, c, existChannel)

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	notExistChannel := a.GetChannelByAddr(addr2)
	assert.Nil(t, notExistChannel, "should be nil for not existed channel.")
}

func subTestRemoveChannelBind(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(proto.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, proto.DefaultLifetime)

	a.RemoveChannelBind(c.Number)

	channelByNumber := a.GetChannelByNumber(c.Number)
	assert.Nil(t, channelByNumber)

	channelByAddr := a.GetChannelByAddr(c.Peer)
	assert.Nil(t, channelByAddr)
}

func subTestAllocationRefresh(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	a.lifetimeTimer = time.AfterFunc(proto.DefaultLifetime, func() {
		wg.Done()
	})
	a.Refresh(0)
	wg.Wait()

	// lifetimeTimer has expired
	assert.False(t, a.lifetimeTimer.Stop())
}

func subTestAllocationClose(t *testing.T) {
	network := "udp"

	l, err := net.ListenPacket(network, "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	a := NewAllocation(nil, nil, nil)
	a.RelaySocket = l
	// add mock lifetimeTimer
	a.lifetimeTimer = time.AfterFunc(proto.DefaultLifetime, func() {})

	// add channel
	addr, _ := net.ResolveUDPAddr(network, "127.0.0.1:3478")
	c := NewChannelBind(proto.MinChannelNumber, addr, nil)
	_ = a.AddChannelBind(c, proto.DefaultLifetime)

	// add permission
	a.AddPermission(NewPermission(addr, nil))

	err = a.Close()
	assert.Nil(t, err, "should succeed")
	assert.True(t, isClose(a.RelaySocket), "should be closed")
}

func subTestPacketHandler(t *testing.T) {
	network := "udp"

	m, _ := newTestManager()

	// turn server initialization
	turnSocket, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	// client listener initialization
	clientListener, err := net.ListenPacket(network, "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	dataCh := make(chan []byte)
	// client listener read data
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

	a, err := m.CreateAllocation(&FiveTuple{
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

	// add permission with peer1 address
	a.AddPermission(NewPermission(peerListener1.LocalAddr(), m.log))
	// add channel with min channel number and peer2 address
	channelBind := NewChannelBind(proto.MinChannelNumber, peerListener2.LocalAddr(), m.log)
	_ = a.AddChannelBind(channelBind, proto.DefaultLifetime)

	_, port, _ := ipnet.AddrIPPort(a.RelaySocket.LocalAddr())
	relayAddrWithHostStr := fmt.Sprintf("127.0.0.1:%d", port)
	relayAddrWithHost, _ := net.ResolveUDPAddr(network, relayAddrWithHostStr)

	// test for permission and data message
	targetText := "permission"
	_, _ = peerListener1.WriteTo([]byte(targetText), relayAddrWithHost)
	data := <-dataCh

	// resolve stun data message
	assert.True(t, stun.IsMessage(data), "should be stun message")

	var msg stun.Message
	err = stun.Decode(data, &msg)
	assert.Nil(t, err, "decode data to stun message failed")

	var msgData proto.Data
	err = msgData.GetFrom(&msg)
	assert.Nil(t, err, "get data from stun message failed")
	assert.Equal(t, targetText, string(msgData), "get message doesn't equal the target text")

	// test for channel bind and channel data
	targetText2 := "channel bind"
	_, _ = peerListener2.WriteTo([]byte(targetText2), relayAddrWithHost)
	data = <-dataCh

	// resolve channel data
	assert.True(t, proto.IsChannelData(data), "should be channel data")

	channelData := proto.ChannelData{
		Raw: data,
	}
	err = channelData.Decode()
	assert.Nil(t, err, fmt.Sprintf("channel data decode with error: %v", err))
	assert.Equal(t, channelBind.Number, channelData.Number, "get channel data's number is invalid")
	assert.Equal(t, targetText2, string(channelData.Data), "get data doesn't equal the target text.")

	// listeners close
	_ = m.Close()
	_ = clientListener.Close()
	_ = peerListener1.Close()
	_ = peerListener2.Close()
}
