// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"net"
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestPeerAddress(t *testing.T) {
	// Simple tests because already tested in stun.
	a := PeerAddress{
		IP:   net.IPv4(111, 11, 1, 2),
		Port: 333,
	}
	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "111.11.1.2:333", a.String())
	})
	m := new(stun.Message)
	assert.NoError(t, a.AddTo(m))

	m.WriteHeader()
	decoded := new(stun.Message)

	_, err := decoded.Write(m.Raw)
	assert.NoError(t, err)

	var aGot PeerAddress
	assert.NoError(t, aGot.GetFrom(decoded))
}
