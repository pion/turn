// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"net"

	"github.com/pion/turn/v4/internal/proto"
)

type STUNConn = proto.STUNConn

// NewSTUNConn creates a STUNConn.
func NewSTUNConn(nextConn net.Conn) *STUNConn {
	return proto.NewSTUNConn(nextConn)
}
