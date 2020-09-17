package proto

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pion/stun"
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
		m := new(stun.Message)
		v := []byte{1, 2, 3, 4}
		if wasAllocs(func() {
			// On stack.
			d := Data(v)
			d.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		d := &Data{1, 2, 3, 4}
		if wasAllocs(func() {
			// On heap.
			d.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		d := Data{1, 2, 33, 44, 0x13, 0xaf}
		if err := d.AddTo(m); err != nil {
			t.Fatal(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			var dataDecoded Data
			if err := dataDecoded.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(dataDecoded, d) {
				t.Error(dataDecoded, "!=", d, "(expected)")
			}
			if wasAllocs(func() {
				var dataDecoded Data
				dataDecoded.GetFrom(decoded) //nolint
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle Data
				if err := handle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
			})
		})
	})
}
