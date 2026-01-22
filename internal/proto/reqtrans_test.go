// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestRequestedTransport(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		transAttr := RequestedTransport{
			Protocol: ProtoUDP,
		}
		assert.Equal(t, "protocol: UDP", transAttr.String())

		transAttr = RequestedTransport{
			Protocol: ProtoTCP,
		}
		assert.Equal(t, "protocol: TCP", transAttr.String())

		transAttr.Protocol = 254
		assert.Equal(t, "protocol: 254", transAttr.String())
	})
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		allocated := wasAllocs(func() {
			// On stack.
			r := RequestedTransport{
				Protocol: ProtoUDP,
			}
			assert.NoError(t, r.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		r := &RequestedTransport{
			Protocol: ProtoUDP,
		}
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, r.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		transAttr := RequestedTransport{
			Protocol: ProtoUDP,
		}
		assert.NoError(t, transAttr.AddTo(m))
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(m.Raw)
			assert.NoError(t, err)

			req := RequestedTransport{
				Protocol: ProtoUDP,
			}
			assert.NoError(t, req.GetFrom(decoded))
			assert.Equal(t, transAttr, req)

			allocated := wasAllocs(func() {
				assert.NoError(t, transAttr.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle RequestedTransport
				assert.ErrorIs(t, handle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrRequestedTransport, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(handle.GetFrom(m)))
			})
		})
	})
}
