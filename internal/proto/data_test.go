// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func BenchmarkData(b *testing.B) {
	b.Run("AddTo", func(b *testing.B) {
		m := new(stun.Message)
		d := make(Data, 10)
		for i := 0; i < b.N; i++ {
			assert.NoError(b, d.AddTo(m))
			m.Reset()
		}
	})
	b.Run("AddToRaw", func(b *testing.B) {
		m := new(stun.Message)
		d := make([]byte, 10)
		// Overhead should be low.
		for i := 0; i < b.N; i++ {
			m.Add(stun.AttrData, d)
			m.Reset()
		}
	})
}

func TestData(t *testing.T) {
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := new(stun.Message)
		v := []byte{1, 2, 3, 4}
		allocated := wasAllocs(func() {
			// On stack.
			d := Data(v)
			assert.NoError(t, d.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		d := &Data{1, 2, 3, 4}
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, d.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		data := Data{1, 2, 33, 44, 0x13, 0xaf}
		assert.NoError(t, data.AddTo(m))

		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(m.Raw)
			assert.NoError(t, err)

			var dataDecoded Data
			assert.NoError(t, dataDecoded.GetFrom(decoded))
			assert.Equal(t, data, dataDecoded)

			allocated := wasAllocs(func() {
				var dataDecoded Data
				assert.NoError(t, dataDecoded.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle Data
				assert.ErrorIs(t, handle.GetFrom(m), stun.ErrAttributeNotFound)
			})
		})
	})
}
