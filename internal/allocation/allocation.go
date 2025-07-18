// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package allocation contains all CRUD operations for allocations
package allocation

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4/internal/ipnet"
	"github.com/pion/turn/v4/internal/proto"
)

type allocationResponse struct {
	transactionID [stun.TransactionIDSize]byte
	responseAttrs []stun.Setter
}

// Allocation is tied to a FiveTuple and relays traffic
// use CreateAllocation and GetAllocation to operate.
type Allocation struct {
	RelayAddr           net.Addr
	Protocol            Protocol
	TurnSocket          net.PacketConn
	RelaySocket         net.PacketConn
	fiveTuple           *FiveTuple
	permissionsLock     sync.RWMutex
	permissions         map[string]*Permission
	channelBindingsLock sync.RWMutex
	channelBindings     []*ChannelBind
	lifetimeTimer       *time.Timer
	closed              chan any
	log                 logging.LeveledLogger

	// Some clients (Firefox or others using resiprocate's nICE lib) may retry allocation
	// with same 5 tuple when received 413, for compatible with these clients,
	// cache for response lost and client retry to implement 'stateless stack approach'
	// See: https://datatracker.ietf.org/doc/html/rfc5766#section-6.2
	responseCache atomic.Value // *allocationResponse
}

// NewAllocation creates a new instance of NewAllocation.
func NewAllocation(turnSocket net.PacketConn, fiveTuple *FiveTuple, log logging.LeveledLogger) *Allocation {
	return &Allocation{
		TurnSocket:  turnSocket,
		fiveTuple:   fiveTuple,
		permissions: make(map[string]*Permission, 64),
		closed:      make(chan any),
		log:         log,
	}
}

// GetPermission gets the Permission from the allocation.
func (a *Allocation) GetPermission(addr net.Addr) *Permission {
	a.permissionsLock.RLock()
	defer a.permissionsLock.RUnlock()

	return a.permissions[ipnet.FingerprintAddr(addr)]
}

// AddPermission adds a new permission to the allocation.
func (a *Allocation) AddPermission(perms *Permission) {
	fingerprint := ipnet.FingerprintAddr(perms.Addr)

	a.permissionsLock.RLock()
	existedPermission, ok := a.permissions[fingerprint]
	a.permissionsLock.RUnlock()

	if ok {
		existedPermission.refresh(permissionTimeout)

		return
	}

	perms.allocation = a
	a.permissionsLock.Lock()
	a.permissions[fingerprint] = perms
	a.permissionsLock.Unlock()

	perms.start(permissionTimeout)
}

// RemovePermission removes the net.Addr's fingerprint from the allocation's permissions.
func (a *Allocation) RemovePermission(addr net.Addr) {
	a.permissionsLock.Lock()
	defer a.permissionsLock.Unlock()
	delete(a.permissions, ipnet.FingerprintAddr(addr))
}

// AddChannelBind adds a new ChannelBind to the allocation, it also updates the
// permissions needed for this ChannelBind.
func (a *Allocation) AddChannelBind(chanBind *ChannelBind, lifetime time.Duration) error {
	// Check that this channel id isn't bound to another transport address, and
	// that this transport address isn't bound to another channel number.
	channelByNumber := a.GetChannelByNumber(chanBind.Number)

	if channelByNumber != a.GetChannelByAddr(chanBind.Peer) {
		return errSameChannelDifferentPeer
	}

	// Add or refresh this channel.
	if channelByNumber == nil {
		a.channelBindingsLock.Lock()
		defer a.channelBindingsLock.Unlock()

		chanBind.allocation = a
		a.channelBindings = append(a.channelBindings, chanBind)
		chanBind.start(lifetime)

		// Channel binds also refresh permissions.
		a.AddPermission(NewPermission(chanBind.Peer, a.log))
	} else {
		channelByNumber.refresh(lifetime)

		// Channel binds also refresh permissions.
		a.AddPermission(NewPermission(channelByNumber.Peer, a.log))
	}

	return nil
}

// RemoveChannelBind removes the ChannelBind from this allocation by id.
func (a *Allocation) RemoveChannelBind(number proto.ChannelNumber) bool {
	a.channelBindingsLock.Lock()
	defer a.channelBindingsLock.Unlock()

	for i := len(a.channelBindings) - 1; i >= 0; i-- {
		if a.channelBindings[i].Number == number {
			a.channelBindings = append(a.channelBindings[:i], a.channelBindings[i+1:]...)

			return true
		}
	}

	return false
}

// GetChannelByNumber gets the ChannelBind from this allocation by id.
func (a *Allocation) GetChannelByNumber(number proto.ChannelNumber) *ChannelBind {
	a.channelBindingsLock.RLock()
	defer a.channelBindingsLock.RUnlock()
	for _, cb := range a.channelBindings {
		if cb.Number == number {
			return cb
		}
	}

	return nil
}

// GetChannelByAddr gets the ChannelBind from this allocation by net.Addr.
func (a *Allocation) GetChannelByAddr(addr net.Addr) *ChannelBind {
	a.channelBindingsLock.RLock()
	defer a.channelBindingsLock.RUnlock()
	for _, cb := range a.channelBindings {
		if ipnet.AddrEqual(cb.Peer, addr) {
			return cb
		}
	}

	return nil
}

// Refresh updates the allocations lifetime.
func (a *Allocation) Refresh(lifetime time.Duration) {
	if !a.lifetimeTimer.Reset(lifetime) {
		a.log.Errorf("Failed to reset allocation timer for %v", a.fiveTuple)
	}
}

// SetResponseCache cache allocation response for retransmit allocation request.
func (a *Allocation) SetResponseCache(transactionID [stun.TransactionIDSize]byte, attrs []stun.Setter) {
	a.responseCache.Store(&allocationResponse{
		transactionID: transactionID,
		responseAttrs: attrs,
	})
}

// GetResponseCache return response cache for retransmit allocation request.
func (a *Allocation) GetResponseCache() (id [stun.TransactionIDSize]byte, attrs []stun.Setter) {
	if res, ok := a.responseCache.Load().(*allocationResponse); ok && res != nil {
		id, attrs = res.transactionID, res.responseAttrs
	}

	return
}

// Close closes the allocation.
func (a *Allocation) Close() error {
	select {
	case <-a.closed:
		return nil
	default:
	}
	close(a.closed)

	a.lifetimeTimer.Stop()

	a.permissionsLock.RLock()
	for _, p := range a.permissions {
		p.lifetimeTimer.Stop()
	}
	a.permissionsLock.RUnlock()

	a.channelBindingsLock.RLock()
	for _, c := range a.channelBindings {
		c.lifetimeTimer.Stop()
	}
	a.channelBindingsLock.RUnlock()

	return a.RelaySocket.Close()
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

const rtpMTU = 1600

func (a *Allocation) packetHandler(manager *Manager) {
	buffer := make([]byte, rtpMTU)

	for {
		n, srcAddr, err := a.RelaySocket.ReadFrom(buffer)
		if err != nil {
			manager.DeleteAllocation(a.fiveTuple)

			return
		}

		a.log.Debugf("Relay socket %s received %d bytes from %s",
			a.RelaySocket.LocalAddr(),
			n,
			srcAddr)

		if channel := a.GetChannelByAddr(srcAddr); channel != nil { // nolint:nestif
			channelData := &proto.ChannelData{
				Data:   buffer[:n],
				Number: channel.Number,
			}
			channelData.Encode()

			if _, err = a.TurnSocket.WriteTo(channelData.Raw, a.fiveTuple.SrcAddr); err != nil {
				a.log.Errorf("Failed to send ChannelData from allocation %v %v", srcAddr, err)
			}
		} else if p := a.GetPermission(srcAddr); p != nil {
			udpAddr, ok := srcAddr.(*net.UDPAddr)
			if !ok {
				a.log.Errorf("Failed to send DataIndication from allocation %v %v", srcAddr, err)

				return
			}

			peerAddressAttr := proto.PeerAddress{IP: udpAddr.IP, Port: udpAddr.Port}
			dataAttr := proto.Data(buffer[:n])

			msg, err := stun.Build(
				stun.TransactionID,
				stun.NewType(stun.MethodData, stun.ClassIndication),
				peerAddressAttr,
				dataAttr,
			)
			if err != nil {
				a.log.Errorf("Failed to send DataIndication from allocation %v %v", srcAddr, err)

				return
			}
			a.log.Debugf("Relaying message from %s to client at %s",
				srcAddr,
				a.fiveTuple.SrcAddr)
			if _, err = a.TurnSocket.WriteTo(msg.Raw, a.fiveTuple.SrcAddr); err != nil {
				a.log.Errorf("Failed to send DataIndication from allocation %v %v", srcAddr, err)
			}
		} else {
			a.log.Infof("No Permission or Channel exists for %v on allocation %v", srcAddr, a.RelayAddr)
		}
	}
}
