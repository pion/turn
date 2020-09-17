package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun"
	"github.com/stretchr/testify/assert"
)

func BenchmarkChannelNumber(b *testing.B) {
	b.Run("AddTo", func(b *testing.B) {
		b.ReportAllocs()
		m := new(stun.Message)
		for i := 0; i < b.N; i++ {
			n := ChannelNumber(12)
			if err := n.AddTo(m); err != nil {
				b.Fatal(err)
			}
			m.Reset()
		}
	})
	b.Run("GetFrom", func(b *testing.B) {
		m := new(stun.Message)
		assert.NoError(b, ChannelNumber(12).AddTo(m))
		for i := 0; i < b.N; i++ {
			var n ChannelNumber
			if err := n.GetFrom(m); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestChannelNumber(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		n := ChannelNumber(112)
		if n.String() != "112" {
			t.Errorf("bad string %s, expedted 112", n.String())
		}
	})
	t.Run("NoAlloc", func(t *testing.T) {
		m := &stun.Message{}
		if wasAllocs(func() {
			// Case with ChannelNumber on stack.
			n := ChannelNumber(6)
			n.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		n := ChannelNumber(12)
		nP := &n
		if wasAllocs(func() {
			// On heap.
			nP.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		n := ChannelNumber(6)
		if err := n.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			var numDecoded ChannelNumber
			if err := numDecoded.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if numDecoded != n {
				t.Errorf("Decoded %d, expected %d", numDecoded, n)
			}
			if wasAllocs(func() {
				var num ChannelNumber
				num.GetFrom(decoded) //nolint
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				nHandle := new(ChannelNumber)
				if err := nHandle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrChannelNumber, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(nHandle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
			})
		})
	})
}

func TestChannelNumber_Valid(t *testing.T) {
	for _, tc := range []struct {
		n     ChannelNumber
		value bool
	}{
		{MinChannelNumber - 1, false},
		{MinChannelNumber, true},
		{MinChannelNumber + 1, true},
		{MaxChannelNumber, true},
		{MaxChannelNumber + 1, false},
	} {
		if v := tc.n.Valid(); v != tc.value {
			t.Errorf("unexpected: (%s) %v != %v", tc.n.String(), tc.value, v)
		}
	}
}
