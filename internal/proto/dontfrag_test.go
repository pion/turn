// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v2"
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
		m := new(stun.Message)
		if err := dontFrag.AddTo(m); err != nil {
			t.Error(err)
		}
		m.WriteHeader()
		t.Run("IsSet", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(m.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			if !dontFrag.IsSet(m) {
				t.Error("should be set")
			}
			if wasAllocs(func() {
				dontFrag.IsSet(m)
			}) {
				t.Error("unexpected allocations")
			}
		})
	})
}
