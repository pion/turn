// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestEvenPort(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		p := EvenPort{}
		assert.Equal(t, "reserve: false", p.String())

		p.ReservePort = true
		assert.Equal(t, "reserve: true", p.String())
	})
	t.Run("False", func(t *testing.T) {
		m := new(stun.Message)
		p := EvenPort{
			ReservePort: false,
		}
		assert.NoError(t, p.AddTo(m))

		m.WriteHeader()
		decoded := new(stun.Message)
		_, err := decoded.Write(m.Raw)
		assert.NoError(t, err)

		var port EvenPort
		assert.NoError(t, port.GetFrom(m))
		assert.Equal(t, p, port)
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		evenPortAttr := EvenPort{
			ReservePort: true,
		}
		assert.NoError(t, evenPortAttr.AddTo(m))
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(m.Raw)
			assert.NoError(t, err)

			port := EvenPort{}
			assert.NoError(t, port.GetFrom(decoded))
			assert.Equalf(t, evenPortAttr, port, "Decoded %q, expected %q", port.String(), evenPortAttr.String())

			allocated := wasAllocs(func() {
				assert.NoError(t, port.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle EvenPort
				assert.ErrorIs(t, handle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrEvenPort, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(handle.GetFrom(m)))
			})
		})
	})
}
