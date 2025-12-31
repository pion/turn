// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"net"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/stretchr/testify/assert"
)

func TestPermission_MarshalUnmarshalBinary(t *testing.T) {
	mockFiveTuple := &FiveTuple{
		Protocol: UDP,
		SrcAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234},
		DstAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5678},
	}
	mockAllocation := NewAllocation(nil, mockFiveTuple, EventHandler{},
		logging.NewDefaultLeveledLoggerForScope("test", logging.LogLevelTrace, nil),
	)

	t.Run("UDP", func(t *testing.T) {
		addr, _ := net.ResolveUDPAddr("udp", "192.168.1.100:1000")
		original := NewPermission(addr, mockAllocation.log, DefaultPermissionTimeout)
		original.allocation = mockAllocation
		original.start(5 * time.Second)

		data, err := original.MarshalBinary()
		assert.NoError(t, err)

		unmarshaled := &Permission{
			allocation: mockAllocation,
			log:        mockAllocation.log,
		}
		err = unmarshaled.UnmarshalBinary(data)
		assert.NoError(t, err)

		// Assertions
		assert.Equal(t, original.Addr.String(), unmarshaled.Addr.String())
		assert.NotNil(t, unmarshaled.lifetimeTimer, "lifetimeTimer should be restarted after unmarshal")
		assert.WithinDuration(
			t,
			original.expiresAt,
			unmarshaled.expiresAt,
			10*time.Millisecond,
			"expiresAt should be preserved",
		)

		// Let timer expire to ensure it was started correctly
		unmarshaled.lifetimeTimer.Stop() // clean up timer
	})

	t.Run("TCP", func(t *testing.T) {
		addr, _ := net.ResolveTCPAddr("tcp", "10.10.10.10:2000")
		original := NewPermission(addr, mockAllocation.log, DefaultPermissionTimeout)
		original.allocation = mockAllocation
		original.start(5 * time.Second)

		data, err := original.MarshalBinary()
		assert.NoError(t, err)

		unmarshaled := &Permission{
			allocation: mockAllocation,
			log:        mockAllocation.log,
		}
		err = unmarshaled.UnmarshalBinary(data)
		assert.NoError(t, err)

		// Assertions
		assert.Equal(t, original.Addr.String(), unmarshaled.Addr.String())
		assert.NotNil(t, unmarshaled.lifetimeTimer, "lifetimeTimer should be restarted after unmarshal")
		assert.WithinDuration(
			t,
			original.expiresAt,
			unmarshaled.expiresAt,
			10*time.Millisecond,
			"expiresAt should be preserved",
		)

		unmarshaled.lifetimeTimer.Stop()
	})
}
