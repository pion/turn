package allocation

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gortc/turn"
	"github.com/pion/stun"
	"github.com/pion/turn/internal/ipnet"
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
		{"GetChannelByID", subTestGetChannelByID},
		{"GetChannelByAddr", subTestGetChannelByAddr},
		{"RemoveChannelBind", subTestRemoveChannelBind},
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
	addr3, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3480")

	p := &Permission{
		Addr: addr,
	}
	p2 := &Permission{
		Addr: addr2,
	}

	a.AddPermission(p)
	a.AddPermission(p2)

	foundP1 := a.GetPermission(addr.String())
	if foundP1 != p {
		t.Error("Got permission is not same as the the added.")
	}

	foundP2 := a.GetPermission(addr2.String())
	if foundP2 != p2 {
		t.Error("Got permission is not same as the the added.")
	}

	foundP3 := a.GetPermission(addr3.String())
	if foundP3 != nil {
		t.Error("Got permission should be nil if not added.")
	}
}

func subTestAddPermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)
	if p.allocation != a {
		t.Error("Permission's allocation should be the adder.")
	}

	foundPermission := a.GetPermission(p.Addr.String())
	if foundPermission != p {
		t.Error("Got permission is not same as the the added.")
	}
}

func subTestRemovePermission(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	p := &Permission{
		Addr: addr,
	}

	a.AddPermission(p)

	foundPermission := a.GetPermission(p.Addr.String())
	if foundPermission != p {
		t.Error("Got permission is not same as the the added.")
	}

	a.RemovePermission(p.Addr.String())

	foundPermission = a.GetPermission(p.Addr.String())
	if foundPermission != nil {
		t.Error("Got permission should be nil after removed.")
	}
}

func subTestAddChannelBind(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(turn.MinChannelNumber, addr, nil)

	err := a.AddChannelBind(c, turn.DefaultLifetime)
	if err != nil {
		t.Error("Add channel should be successful but failed. ")
	}

	if c.allocation != a {
		t.Error("Added channel's allocation should be the caller.")
	}

	c2 := NewChannelBind(turn.MinChannelNumber+1, addr, nil)
	err = a.AddChannelBind(c2, turn.DefaultLifetime)
	if err == nil {
		t.Error("Add channel should fail with conflicted peer address.")
	}

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	c3 := NewChannelBind(turn.MinChannelNumber, addr2, nil)
	err = a.AddChannelBind(c3, turn.DefaultLifetime)
	if err == nil {
		t.Error("Add channel should fail with conflicted number.")
	}
}

func subTestGetChannelByID(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(turn.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, turn.DefaultLifetime)

	existChannel := a.GetChannelByID(c.Number)
	if existChannel != c {
		t.Error("Got channel should not be nil for an existed channel.")
	}

	notExistChannel := a.GetChannelByID(turn.MinChannelNumber + 1)
	if notExistChannel != nil {
		t.Error("Got channel should be nil for a not existed channel.")
	}
}

func subTestGetChannelByAddr(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(turn.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, turn.DefaultLifetime)

	existChannel := a.GetChannelByAddr(c.Peer.String())
	if existChannel != c {
		t.Error("Got channel should not be nil for an existed channel.")
	}

	addr2, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3479")
	notExistChannel := a.GetChannelByAddr(addr2.String())
	if notExistChannel != nil {
		t.Error("Got channel should be nil for a not existed channel.")
	}
}

func subTestRemoveChannelBind(t *testing.T) {
	a := NewAllocation(nil, nil, nil)

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(turn.MinChannelNumber, addr, nil)

	_ = a.AddChannelBind(c, turn.DefaultLifetime)

	a.RemoveChannelBind(c)

	channelByID := a.GetChannelByID(c.Number)
	if channelByID != nil {
		t.Error("Got channel should be nil after removed.")
	}

	channelByAddr := a.GetChannelByAddr(c.Peer.String())
	if channelByAddr != nil {
		t.Error("Got channel should be nil after removed.")
	}
}

func subTestAllocationClose(t *testing.T) {
	l, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		panic(err)
	}

	a := NewAllocation(nil, nil, nil)
	a.RelaySocket = l
	// add mock lifetimeTimer
	a.lifetimeTimer = time.AfterFunc(turn.DefaultLifetime, func() {})

	// add channel
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:3478")
	c := NewChannelBind(turn.MinChannelNumber, addr, nil)
	_ = a.AddChannelBind(c, turn.DefaultLifetime)

	// add permission
	a.AddPermission(NewPermission(addr, nil))

	if err = a.Close(); err != nil {
		t.Error("Close allocation should succeed but fail.")
	}

	if !isClose(a.RelaySocket) {
		t.Error("Allocation's relay socket close failed.")
	}
}

func subTestPacketHandler(t *testing.T) {
	m := newTestManager()

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
	}, turnSocket, 0, turn.DefaultLifetime)

	if err != nil {
		t.Error("Create allocation should succeed but fail.")
	}

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
	channelBind := NewChannelBind(turn.MinChannelNumber, peerListener2.LocalAddr(), m.log)
	_ = a.AddChannelBind(channelBind, turn.DefaultLifetime)

	_, port, _ := ipnet.AddrIPPort(a.RelaySocket.LocalAddr())
	relayAddrWithHostStr := fmt.Sprintf("127.0.0.1:%d", port)
	relayAddrWithHost, _ := net.ResolveUDPAddr(network, relayAddrWithHostStr)

	// test for permission and data message
	targetText := "permission"
	_, _ = peerListener1.WriteTo([]byte(targetText), relayAddrWithHost)
	data := <-dataCh

	// resolve stun data message
	if !stun.IsMessage(data) {
		t.Error("Read data should be stun message.")
	}

	var msg stun.Message
	err = stun.Decode(data, &msg)
	if err != nil {
		t.Error("Decode data message failed.")
	}

	var msgData turn.Data
	if err := msgData.GetFrom(&msg); err != nil {
		t.Error("Get message data failed.")
	}

	if string(msgData) != targetText {
		t.Error("Get message doesn't equal the target text.")
	}

	// test for channel bind and channel data
	targetText2 := "channel bind"
	_, _ = peerListener2.WriteTo([]byte(targetText2), relayAddrWithHost)
	data = <-dataCh

	// resolve channel data
	if !turn.IsChannelData(data) {
		t.Error("Read data should be channel data.")
	}

	channelData := turn.ChannelData{
		Raw: data,
	}

	if err := channelData.Decode(); err != nil {
		t.Errorf("Channel data decode with error: %v.", err)
	}

	if channelData.Number != channelBind.Number {
		t.Errorf("Get channel data's number is invalid, expect %d, but %d.", channelBind.Number, channelData.Number)
	}

	if string(channelData.Data) != targetText2 {
		t.Error("Get data doesn't equal the target text.")
	}

	// listeners close
	_ = m.Close()
	_ = clientListener.Close()
	_ = peerListener1.Close()
	_ = peerListener2.Close()
}
