// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestReservationToken(t *testing.T) {
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		tok := make([]byte, 8)
		allocated := wasAllocs(func() {
			// On stack.
			tk := ReservationToken(tok)
			assert.NoError(t, tk.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		tk := make(ReservationToken, 8)
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, tk.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		tk := make(ReservationToken, 8)
		tk[2] = 33
		tk[7] = 1
		assert.NoError(t, tk.AddTo(stunMsg))

		stunMsg.WriteHeader()
		t.Run("HandleErr", func(t *testing.T) {
			badTk := ReservationToken{34, 45}
			assert.True(t, stun.IsAttrSizeInvalid(badTk.AddTo(stunMsg)))
		})
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(stunMsg.Raw)
			assert.NoError(t, err)

			var tok ReservationToken
			assert.NoError(t, tok.GetFrom(decoded))
			assert.Equal(t, tk, tok)
			allocated := wasAllocs(func() {
				assert.NoError(t, tok.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle ReservationToken
				assert.ErrorIs(t, handle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrReservationToken, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(handle.GetFrom(m)))
			})
		})
	})
}
