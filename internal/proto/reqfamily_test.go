package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun"
)

func TestRequestedAddressFamily(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		if RequestedFamilyIPv4.String() != "IPv4" {
			t.Errorf("bad string %q, expected %q", RequestedFamilyIPv4,
				"IPv4",
			)
		}
		if RequestedFamilyIPv6.String() != "IPv6" {
			t.Errorf("bad string %q, expected %q", RequestedFamilyIPv6,
				"IPv6",
			)
		}
		if RequestedAddressFamily(0x04).String() != "unknown" {
			t.Error("should be unknown")
		}
	})
	t.Run("NoAlloc", func(t *testing.T) {
		m := &stun.Message{}
		if wasAllocs(func() {
			// On stack.
			r := RequestedFamilyIPv4
			r.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		r := new(RequestedAddressFamily)
		*r = RequestedFamilyIPv4
		if wasAllocs(func() {
			// On heap.
			r.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		r := RequestedFamilyIPv4
		if err := r.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			var req RequestedAddressFamily
			if err := req.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if req != r {
				t.Errorf("Decoded %q, expected %q", req, r)
			}
			if wasAllocs(func() {
				r.GetFrom(decoded) //nolint
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle RequestedAddressFamily
				if err := handle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrRequestedAddressFamily, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(handle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
				m.Reset()
				m.Add(stun.AttrRequestedAddressFamily, []byte{5, 0, 0, 0})
				if handle.GetFrom(m) == nil {
					t.Error("should error on invalid value")
				}
			})
		})
	})
}
