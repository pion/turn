// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// Package offload implements a kernel-offload engine to speed up transporting ChannelData messages
//
//nolint:gochecknoinits
package offload

import (
	"fmt"
	"net"

	"github.com/pion/logging"
	"github.com/pion/turn/v3/internal/proto"
)

// Engine represents the network offloading engine
//
//nolint:gochecknoglobals
var Engine OffloadEngine

// Init Engine as NullOffload
func init() {
	log := logging.NewDefaultLoggerFactory().NewLogger("offload")
	Engine, _ = NewNullEngine(log)
}

// OffloadEngine provides a general interface for offloading techniques (e.g., XDP)
//
//nolint:revive
type OffloadEngine interface {
	Init() error
	Shutdown()
	Upsert(client, peer Connection) error
	Remove(client, peer Connection) error
	List() (map[Connection]Connection, error)
}

// Connection combines offload engine identifiers required for uinquely identifying allocation channel bindings. Depending of the used offload engine, some values are not required. For example, the SockFd has no role for an XDP offload
type Connection struct {
	RemoteAddr net.Addr
	LocalAddr  net.Addr
	Protocol   proto.Protocol
	SocketFd   uintptr
	ChannelID  uint32
}

func (c *Connection) String() string {
	return fmt.Sprintf("%s:local:%s-remote:%s-chan:%d",
		c.RemoteAddr.Network(), c.LocalAddr.String(), c.RemoteAddr.String(),
		c.ChannelID)
}
