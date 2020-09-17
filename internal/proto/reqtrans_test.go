package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun"
)

func TestRequestedTransport(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		r := RequestedTransport{
			Protocol: ProtoUDP,
		}
		if r.String() != "protocol: UDP" {
			t.Errorf("bad string %q, expected %q", r,
				"protocol: UDP",
			)
		}
		r.Protocol = 254
		if r.String() != "protocol: 254" {
			if r.String() != "protocol: UDP" {
				t.Errorf("bad string %q, expected %q", r,
					"protocol: 254",
				)
			}
		}
	})
	t.Run("NoAlloc", func(t *testing.T) {
		m := &stun.Message{}
		if wasAllocs(func() {
			// On stack.
			r := RequestedTransport{
				Protocol: ProtoUDP,
			}
			r.AddTo(m) //nolint
			m.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		r := &RequestedTransport{
			Protocol: ProtoUDP,
		}
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
		r := RequestedTransport{
			Protocol: ProtoUDP,
		}
		if err := r.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			req := RequestedTransport{
				Protocol: ProtoUDP,
			}
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
				var handle RequestedTransport
				if err := handle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrRequestedTransport, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(handle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
			})
		})
	})
}
