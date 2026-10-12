// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"net"

	"github.com/pion/turn/v5/internal/proto"
)

// STUNConn frames STUN messages on top of a stream-oriented connection.
type STUNConn = proto.STUNConn

// NewSTUNConn creates a STUNConn over a stream-oriented connection such as TCP,
// TLS, or DTLS.
//
// Do not wrap UDP connections with NewSTUNConn. UDP already preserves datagram
// boundaries and should be used through net.PacketConn directly.
func NewSTUNConn(nextConn net.Conn) *STUNConn {
	return proto.NewSTUNConn(nextConn)
}
