// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun/v3"
)

func TestRequestedTransport(t *testing.T) { //nolint:cyclop
	t.Run("String", func(t *testing.T) {
		transAttr := RequestedTransport{
			Protocol: ProtoUDP,
		}
		if transAttr.String() != "protocol: UDP" {
			t.Errorf("bad string %q, expected %q", transAttr,
				"protocol: UDP",
			)
		}
		transAttr = RequestedTransport{
			Protocol: ProtoTCP,
		}
		if transAttr.String() != "protocol: TCP" {
			t.Errorf("bad string %q, expected %q", transAttr,
				"protocol: TCP",
			)
		}
		transAttr.Protocol = 254
		if transAttr.String() != "protocol: 254" {
			t.Errorf("bad string %q, expected %q", transAttr,
				"protocol: 254",
			)
		}
	})
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		if wasAllocs(func() {
			// On stack.
			r := RequestedTransport{
				Protocol: ProtoUDP,
			}
			r.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		r := &RequestedTransport{
			Protocol: ProtoUDP,
		}
		if wasAllocs(func() {
			// On heap.
			r.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		m := new(stun.Message)
		transAttr := RequestedTransport{
			Protocol: ProtoUDP,
		}
		if err := transAttr.AddTo(m); err != nil {
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
			if req != transAttr {
				t.Errorf("Decoded %q, expected %q", req, transAttr)
			}
			if wasAllocs(func() {
				transAttr.GetFrom(decoded) //nolint
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
