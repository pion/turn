package proto

import (
	"testing"

	"fmt"
	"time"

	"github.com/pion/stun"
)

func ExampleLifetime() {
	// Encoding lifetime to message.
	m := new(stun.Message)
	Lifetime{time.Minute}.AddTo(m)
	m.WriteHeader()

	// Decoding message.
	mDec := new(stun.Message)
	if _, err := m.WriteTo(mDec); err != nil {
		panic(err)
	}
	// Decoding lifetime from message.
	l := Lifetime{}
	l.GetFrom(m)
	fmt.Println("Decoded:", l)
	// Output:
	// Decoded: 1m0s
}

func BenchmarkLifetime(b *testing.B) {
	b.Run("AddTo", func(b *testing.B) {
		b.ReportAllocs()
		m := new(stun.Message)
		for i := 0; i < b.N; i++ {
			l := Lifetime{time.Second}
			if err := l.AddTo(m); err != nil {
				b.Fatal(err)
			}
			m.Reset()
		}
	})
	b.Run("GetFrom", func(b *testing.B) {
		m := new(stun.Message)
		Lifetime{time.Minute}.AddTo(m)
		for i := 0; i < b.N; i++ {
			l := Lifetime{}
			if err := l.GetFrom(m); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestLifetime(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		l := Lifetime{time.Second * 10}
		if l.String() != "10s" {
			t.Errorf("bad string %s, expedted 10s", l)
		}
	})
	t.Run("NoAlloc", func(t *testing.T) {
		m := &stun.Message{}
		if wasAllocs(func() {
			// On stack.
			l := Lifetime{
				Duration: time.Minute,
			}
			l.AddTo(m)
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		l := &Lifetime{time.Second}
		if wasAllocs(func() {
			// On heap.
			l.AddTo(m)
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		l := Lifetime{time.Second * 10}
		if err := l.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			life := Lifetime{}
			if err := life.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if life != l {
				t.Errorf("Decoded %q, expected %q", life, l)
			}
			if wasAllocs(func() {
				life.GetFrom(decoded)
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				nHandle := new(Lifetime)
				if err := nHandle.GetFrom(m); err != stun.ErrAttributeNotFound {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrLifetime, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(nHandle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
			})
		})
	})
}
