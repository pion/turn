// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v4"
	"github.com/stretchr/testify/assert"
)

func BenchmarkChannelNumber(b *testing.B) {
	b.Run("AddTo", func(b *testing.B) {
		b.ReportAllocs()
		m := new(stun.Message)
		for i := 0; i < b.N; i++ {
			n := ChannelNumber(12)
			assert.NoError(b, n.AddTo(m))
			m.Reset()
		}
	})
	b.Run("GetFrom", func(b *testing.B) {
		m := new(stun.Message)
		assert.NoError(b, ChannelNumber(12).AddTo(m))
		for i := 0; i < b.N; i++ {
			var n ChannelNumber
			assert.NoError(b, n.GetFrom(m))
		}
	})
}

func TestChannelNumber(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		n := ChannelNumber(112)
		assert.Equal(t, "112", n.String())
	})
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		allocated := wasAllocs(func() {
			// Case with ChannelNumber on stack.
			n := ChannelNumber(6)
			assert.NoError(t, n.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)

		n := ChannelNumber(12)
		nP := &n
		allocated = wasAllocs(func() {
			// On heap.
			assert.NoError(t, nP.AddTo(stunMsg))
			stunMsg.Reset()
		})
		assert.False(t, allocated)
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		chanNumber := ChannelNumber(6)
		assert.NoError(t, chanNumber.AddTo(stunMsg))

		stunMsg.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(stunMsg.Raw)
			assert.NoError(t, err)

			var numDecoded ChannelNumber
			err = numDecoded.GetFrom(decoded)
			assert.NoError(t, err)
			assert.Equal(t, chanNumber, numDecoded)

			allocated := wasAllocs(func() {
				var num ChannelNumber
				assert.NoError(t, num.GetFrom(decoded))
			})
			assert.False(t, allocated)

			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				nHandle := new(ChannelNumber)
				assert.ErrorIs(t, nHandle.GetFrom(m), stun.ErrAttributeNotFound)

				m.Add(stun.AttrChannelNumber, []byte{1, 2, 3})
				assert.True(t, stun.IsAttrSizeInvalid(nHandle.GetFrom(m)))
			})
		})
	})
}

// maxChannelNumberRFC8656 is an helper constant for channelNumberRangeRFC8656.
const maxChannelNumberRFC8656 = 0x4FFF

// channelNumberRangeRFC8656 is an helper function which is used to make sure
// that the only range covered is not only 0x4000-0x4FFF.
func channelNumberRangeRFC8656() ChannelNumberRange {
	return ChannelNumberRange{0x4000, 0x4FFF, nil}
}

func TestChannelNumberRange_Contains(t *testing.T) {
	for _, tc := range []struct {
		n     ChannelNumber
		r     ChannelNumberRange
		value bool
	}{
		{MinChannelNumber - 1, ChannelNumberRangeRFC5766(), false},
		{MinChannelNumber - 1, channelNumberRangeRFC8656(), false},
		{MinChannelNumber, ChannelNumberRangeRFC5766(), true},
		{MinChannelNumber, channelNumberRangeRFC8656(), true},
		{MinChannelNumber + 1, ChannelNumberRangeRFC5766(), true},
		{MinChannelNumber + 1, channelNumberRangeRFC8656(), true},
		{MaxChannelNumber, ChannelNumberRangeRFC5766(), true},
		{maxChannelNumberRFC8656, channelNumberRangeRFC8656(), true},
		{MaxChannelNumber + 1, ChannelNumberRangeRFC5766(), false},
		{maxChannelNumberRFC8656 + 1, channelNumberRangeRFC8656(), false},
	} {
		v := tc.r.Contains(tc.n)
		assert.Equalf(t, tc.value, v, "unexpected: (%s) %v != %v", tc.n.String(), tc.value, v)
	}
}
