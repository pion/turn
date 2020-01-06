package proto

import (
	"testing"

	"github.com/pion/stun"
)

func TestDontFragment(t *testing.T) {
	var DontFragment DontFragmentAttr

	t.Run("False", func(t *testing.T) {
		m := new(stun.Message)
		m.WriteHeader()
		if DontFragment.IsSet(m) {
			t.Error("should not be set")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		if err := DontFragment.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("IsSet", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			if !DontFragment.IsSet(m) {
				t.Error("should be set")
			}
			if wasAllocs(func() {
				DontFragment.IsSet(m)
			}) {
				t.Error("unexpected allocations")
			}
		})
	})
}
