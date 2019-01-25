package allocation

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pions/stun"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

// Allocation is tied to a FiveTuple and relays traffic
// use CreateAllocation and GetAllocation to operate
type Allocation struct {
	RelayAddr *stun.TransportAddr
	Protocol  Protocol

	TurnSocket  *ipv4.PacketConn
	RelaySocket *ipv4.PacketConn

	fiveTuple *FiveTuple

	permissionsLock sync.RWMutex
	permissions     []*Permission

	channelBindingsLock sync.RWMutex
	channelBindings     []*ChannelBind

	lifetimeTimer *time.Timer
}

// AddPermission adds a new permission to the allocation
func (a *Allocation) AddPermission(p *Permission) {
	a.permissionsLock.Lock()
	defer a.permissionsLock.Unlock()
	for _, existingPermission := range a.permissions {
		if p.Addr.Equal(existingPermission.Addr) {
			existingPermission.refresh()
			return
		}
	}

	p.allocation = a
	a.permissions = append(a.permissions, p)
	p.start()
}

// RemovePermission removes the TransportAddr from the allocation's permissions
func (a *Allocation) RemovePermission(addr *stun.TransportAddr) bool {
	a.permissionsLock.Lock()
	defer a.permissionsLock.Unlock()

	for i := len(a.permissions) - 1; i >= 0; i-- {
		if a.permissions[i].Addr.Equal(addr) {
			a.permissions = append(a.permissions[:i], a.permissions[i+1:]...)
			return true
		}
	}

	return false
}

// GetPermission gets the Permission from the allocation
func (a *Allocation) GetPermission(addr *stun.TransportAddr) *Permission {
	a.permissionsLock.RLock()
	defer a.permissionsLock.RUnlock()
	for _, p := range a.permissions {
		if p.Addr.Equal(addr) {
			return p
		}
	}
	return nil
}

// AddChannelBind adds a new ChannelBind to the allocation, it also updates the
// permissions needed for this ChannelBind
func (a *Allocation) AddChannelBind(c *ChannelBind) error {
	// Check that this channel id isn't bound to another transport address, and
	// that this transport address isn't bound to another channel id.
	channelByID := a.GetChannelByID(c.ID)
	channelByPeer := a.GetChannelByAddr(c.Peer)
	if channelByID != channelByPeer {
		return errors.Errorf("You cannot use the same channel number with different peer")
	}

	// Add or refresh this channel.
	if channelByID == nil {
		a.channelBindingsLock.Lock()
		defer a.channelBindingsLock.Unlock()

		c.allocation = a
		a.channelBindings = append(a.channelBindings, c)
		c.start()

		// Channel binds also refresh permissions.
		a.AddPermission(&Permission{Addr: c.Peer})
	} else {
		channelByID.refresh()

		// Channel binds also refresh permissions.
		a.AddPermission(&Permission{Addr: channelByID.Peer})
	}

	return nil
}

// RemoveChannelBind removes the ChannelBind from this allocation by id
func (a *Allocation) RemoveChannelBind(id uint16) bool {
	a.channelBindingsLock.Lock()
	defer a.channelBindingsLock.Unlock()

	for i := len(a.channelBindings) - 1; i >= 0; i-- {
		if a.channelBindings[i].ID == id {
			a.channelBindings = append(a.channelBindings[:i], a.channelBindings[i+1:]...)
			return true
		}
	}

	return false
}

// GetChannelByID gets the ChannelBind from this allocation by id
func (a *Allocation) GetChannelByID(id uint16) *ChannelBind {
	a.channelBindingsLock.RLock()
	defer a.channelBindingsLock.RUnlock()
	for _, cb := range a.channelBindings {
		if cb.ID == id {
			return cb
		}
	}
	return nil
}

// GetChannelByAddr gets the ChannelBind from this allocation by stun.TransportAddr
func (a *Allocation) GetChannelByAddr(addr *stun.TransportAddr) *ChannelBind {
	a.channelBindingsLock.RLock()
	defer a.channelBindingsLock.RUnlock()
	for _, cb := range a.channelBindings {
		if cb.Peer.Equal(addr) {
			return cb
		}
	}
	return nil
}

// Refresh updates the allocations lifetime
func (a *Allocation) Refresh(lifetime uint32) {
	if lifetime == 0 {
		if !a.lifetimeTimer.Stop() {
			fmt.Printf("Failed to stop allocation timer for %v \n", a.fiveTuple)
		}
		return
	}
	if !a.lifetimeTimer.Reset(time.Duration(lifetime) * time.Second) {
		fmt.Printf("Failed to reset allocation timer for %v \n", a.fiveTuple)
	}
}

//  https://tools.ietf.org/html/rfc5766#section-10.3
//  When the server receives a UDP datagram at a currently allocated
//  relayed transport address, the server looks up the allocation
//  associated with the relayed transport address.  The server then
//  checks to see whether the set of permissions for the allocation allow
//  the relaying of the UDP datagram as described in Section 8.
//
//  If relaying is permitted, then the server checks if there is a
//  channel bound to the peer that sent the UDP datagram (see
//  Section 11).  If a channel is bound, then processing proceeds as
//  described in Section 11.7.
//
//  If relaying is permitted but no channel is bound to the peer, then
//  the server forms and sends a Data indication.  The Data indication
//  MUST contain both an XOR-PEER-ADDRESS and a DATA attribute.  The DATA
//  attribute is set to the value of the 'data octets' field from the
//  datagram, and the XOR-PEER-ADDRESS attribute is set to the source
//  transport address of the received UDP datagram.  The Data indication
//  is then sent on the 5-tuple associated with the allocation.
func (a *Allocation) packetHandler(m *Manager) {
	const RtpMTU = 1500
	buffer := make([]byte, RtpMTU)

	for {
		n, cm, srcAddr, err := a.RelaySocket.ReadFrom(buffer)
		if err != nil {
			if !m.DeleteAllocation(a.fiveTuple) {
				fmt.Println("Failed to remove allocation after relay listener had closed")
			}
			return
		}

		if channel := a.GetChannelByAddr(&stun.TransportAddr{IP: cm.Dst, Port: a.RelayAddr.Port}); channel != nil {
			channelData := make([]byte, 4)
			binary.BigEndian.PutUint16(channelData[0:], channel.ID)
			binary.BigEndian.PutUint16(channelData[2:], uint16(n))
			channelData = append(channelData, buffer[:n]...)

			if _, err = a.TurnSocket.WriteTo(channelData, nil, a.fiveTuple.SrcAddr.Addr()); err != nil {
				fmt.Printf("Failed to send ChannelData from allocation %v %v \n", srcAddr, err)
			}
		} else if p := a.GetPermission(&stun.TransportAddr{IP: srcAddr.(*net.UDPAddr).IP, Port: srcAddr.(*net.UDPAddr).Port}); p != nil {
			dataAttr := stun.Data{Data: buffer[:n]}
			xorPeerAddressAttr := stun.XorPeerAddress{XorAddress: stun.XorAddress{IP: srcAddr.(*net.UDPAddr).IP, Port: srcAddr.(*net.UDPAddr).Port}}
			if err = stun.BuildAndSend(a.TurnSocket, a.fiveTuple.SrcAddr, stun.ClassIndication, stun.MethodData, stun.GenerateTransactionID(), &xorPeerAddressAttr, &dataAttr); err != nil {
				fmt.Printf("Failed to send DataIndication from allocation %v %v \n", srcAddr, err)
			}
		} else {
			fmt.Printf("Packet unhandled in relay src %v \n", srcAddr)
		}
	}

}
