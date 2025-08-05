// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
)

// EventHandler is a set of callbacks that the server will call at certain hook points during an
// allocation's lifecycle. All events are reported with the context that identifies the allocation
// triggering the event (source and destination address, protocol, username and realm used for
// authenticating the allocation), plus additional callback specific parameters. It is OK to handle
// only a subset of the callbacks.
type EventHandler struct {
	// OnAuth is called after an authentication request has been processed with the TURN method
	// triggering the authentication request (either "Allocate", "Refresh" "CreatePermission",
	// or "ChannelBind"), and the verdict is the authentication result.
	OnAuth func(srcAddr, dstAddr net.Addr, protocol, username, realm string, method string, verdict bool)
	// OnAllocationCreated is called after a new allocation has been made. The relayAddr
	// argument specifies the relay address and requestedPort is the port requested by the
	// client (if any).
	OnAllocationCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, requestedPort int)
	// OnAllocationDeleted is called after an allocation has been removed.
	OnAllocationDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string)
	// OnAllocationError is called when the readloop hdndling an allocation exits with an
	// error with an error message.
	OnAllocationError func(srcAddr, dstAddr net.Addr, protocol, message string)
	// OnPermissionCreated is called after a new permission has been made to an IP address.
	OnPermissionCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, peer net.IP)
	// OnPermissionDeleted is called after a permission for a given IP address has been
	// removed.
	OnPermissionDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr net.Addr, peer net.IP)
	// OnChannelCreated is called after a new channel has been made. The relay address, the
	// peer address and the channel number can be used to uniquely identify the channel
	// created.
	OnChannelCreated func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr, peer net.Addr, channelNumber uint16)
	// OnChannelDeleted is called after a channel has been removed from the server. The relay
	// address, the peer address and the channel number can be used to uniquely identify the
	// channel deleted.
	OnChannelDeleted func(srcAddr, dstAddr net.Addr, protocol, username, realm string,
		relayAddr, peer net.Addr, channelNumber uint16)
}
