// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
)

// EventHandlerType is a type for signaling low-level event callbacks to the server.
type EventHandlerType int

// Event handler types.
const (
	UnknownEvent EventHandlerType = iota
	OnAuth
	OnAllocationCreated
	OnAllocationDeleted
	OnAllocationError
	OnPermissionCreated
	OnPermissionDeleted
	OnChannelCreated
	OnChannelDeleted
)

// EventHandlerArgs is a set of arguments passed from the low-level event callbacks to the server.
type EventHandlerArgs struct {
	Type                                  EventHandlerType
	SrcAddr, DstAddr, RelayAddr, PeerAddr net.Addr
	Protocol                              Protocol
	Username, Realm, Method, Message      string
	Verdict                               bool
	RequestedPort                         int
	PeerIP                                net.IP
	ChannelNumber                         uint16
}

// EventHandler is a callback used by the server to surface allocation lifecycle events.
type EventHandler func(EventHandlerArgs)
