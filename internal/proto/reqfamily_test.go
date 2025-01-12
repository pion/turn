// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"errors"
	"testing"

	"github.com/pion/stun/v3"
)

func TestRequestedAddressFamily(t *testing.T) { //nolint:cyclop
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
		stunMsg := &stun.Message{}
		if wasAllocs(func() {
			// On stack.
			r := RequestedFamilyIPv4
			r.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		requestFamilyAttr := new(RequestedAddressFamily)
		*requestFamilyAttr = RequestedFamilyIPv4
		if wasAllocs(func() {
			// On heap.
			requestFamilyAttr.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		requestFamilyAddr := RequestedFamilyIPv4
		if err := requestFamilyAddr.AddTo(stunMsg); err != nil {
			t.Error(err)
		}
		stunMsg.WriteHeader()
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(stunMsg.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			var req RequestedAddressFamily
			if err := req.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if req != requestFamilyAddr {
				t.Errorf("Decoded %q, expected %q", req, requestFamilyAddr)
			}
			if wasAllocs(func() {
				requestFamilyAddr.GetFrom(decoded) //nolint
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
