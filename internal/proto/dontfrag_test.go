// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
)

func TestDontFragment(t *testing.T) {
	var dontFrag DontFragment

	t.Run("False", func(t *testing.T) {
		m := new(stun.Message)
		m.WriteHeader()
		if dontFrag.IsSet(m) {
			t.Error("should not be set")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		if err := dontFrag.AddTo(stunMsg); err != nil {
			t.Error(err)
		}
		stunMsg.WriteHeader()
		t.Run("IsSet", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(stunMsg.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			if !dontFrag.IsSet(stunMsg) {
				t.Error("should be set")
			}
			if wasAllocs(func() {
				dontFrag.IsSet(stunMsg)
			}) {
				t.Error("unexpected allocations")
			}
		})
	})
}
