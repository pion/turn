// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func BenchmarkLifetime(b *testing.B) {
	b.Run("AddTo", func(b *testing.B) {
		b.ReportAllocs()
		m := new(stun.Message)
		for i := 0; i < b.N; i++ {
			l := Lifetime{time.Second}
			assert.NoError(b, l.AddTo(m))
			m.Reset()
		}
	})
	b.Run("GetFrom", func(b *testing.B) {
		m := new(stun.Message)
		assert.NoError(b, Lifetime{time.Minute}.AddTo(m))
		for i := 0; i < b.N; i++ {
			l := Lifetime{}
			assert.NoError(b, l.GetFrom(m))
		}
	})
}

func TestLifetime(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		l := Lifetime{time.Second * 10}
		assert.Equal(t, "10s", l.String())
	})
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		allocated := wasAllocs(func() {
			// On stack.
			l := Lifetime{
				Duration: time.Minute,
			}
			assert.NoError(t, l.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		l := &Lifetime{time.Second}
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, l.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		lifetime := Lifetime{time.Second * 10}
		assert.NoError(t, lifetime.AddTo(m))
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(m.Raw)
			assert.NoError(t, err)

			life := Lifetime{}
			assert.NoError(t, life.GetFrom(decoded))
			assert.Equal(t, lifetime, life)

			allocated := wasAllocs(func() {
				assert.NoError(t, life.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				nHandle := new(Lifetime)
				assert.ErrorIs(t, nHandle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrLifetime, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(nHandle.GetFrom(m)))
			})
		})
	})
}
