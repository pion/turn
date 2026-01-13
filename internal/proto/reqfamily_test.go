// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestRequestedAddressFamily(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "IPv4", RequestedFamilyIPv4.String())
		assert.Equal(t, "IPv6", RequestedFamilyIPv6.String())
		assert.Equal(t, "unknown", RequestedAddressFamily(0x04).String())
		assert.Equal(t, "unknown", RequestedAddressFamily(0x00).String())
	})
	t.Run("Values", func(t *testing.T) {
		assert.Equal(t, byte(0x01), byte(RequestedFamilyIPv4))
		assert.Equal(t, byte(0x02), byte(RequestedFamilyIPv6))
	})
	t.Run("IPv6", func(t *testing.T) {
		stunMsg := new(stun.Message)
		requestFamilyAddr := RequestedFamilyIPv6
		assert.NoError(t, requestFamilyAddr.AddTo(stunMsg))

		stunMsg.WriteHeader()
		decoded := new(stun.Message)
		_, err := decoded.Write(stunMsg.Raw)
		assert.NoError(t, err)

		var req RequestedAddressFamily
		assert.NoError(t, req.GetFrom(decoded))
		assert.Equal(t, RequestedFamilyIPv6, req)
		assert.Equal(t, "IPv6", req.String())
	})
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		allocated := wasAllocs(func() {
			// On stack.
			r := RequestedFamilyIPv4
			assert.NoError(t, r.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		requestFamilyAttr := new(RequestedAddressFamily)
		*requestFamilyAttr = RequestedFamilyIPv4
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, requestFamilyAttr.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		requestFamilyAddr := RequestedFamilyIPv4
		assert.NoError(t, requestFamilyAddr.AddTo(stunMsg))

		stunMsg.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(stunMsg.Raw)
			assert.NoError(t, err)

			var req RequestedAddressFamily
			assert.NoError(t, req.GetFrom(decoded))
			assert.Equal(t, requestFamilyAddr, req)

			allocated := wasAllocs(func() {
				assert.NoError(t, requestFamilyAddr.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle RequestedAddressFamily
				assert.ErrorIs(t, handle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrRequestedAddressFamily, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(handle.GetFrom(m)))

				m.Reset()
				m.Add(stun.AttrRequestedAddressFamily, []byte{5, 0, 0, 0})
				assert.NotNil(t, handle.GetFrom(m), "should not error on unknown value")
			})
		})
	})
}
