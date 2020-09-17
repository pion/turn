package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun"
	"github.com/stretchr/testify/assert"
)

func TestEvenPort(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		p := EvenPort{}
		if p.String() != "reserve: false" {
			t.Errorf("bad value %q for reselve: false", p.String())
		}
		p.ReservePort = true
		if p.String() != "reserve: true" {
			t.Errorf("bad value %q for reselve: true", p.String())
		}
	})
	t.Run("False", func(t *testing.T) {
		m := new(stun.Message)
		p := EvenPort{
			ReservePort: false,
		}
		if err := p.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		decoded := new(stun.Message)
		var port EvenPort
		_, err := decoded.Write(m.Raw)
		assert.NoError(t, err)
		assert.NoError(t, port.GetFrom(m))
		if port != p {
			t.Fatal("not equal")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		p := EvenPort{
			ReservePort: true,
		}
		if err := p.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			port := EvenPort{}
			if err := port.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if port != p {
				t.Errorf("Decoded %q, expected %q", port.String(), p.String())
			}
			if wasAllocs(func() {
				port.GetFrom(decoded) //nolint
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle EvenPort
				if err := handle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrEvenPort, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(handle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
			})
		})
	})
}
