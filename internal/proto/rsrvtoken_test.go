// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pion/stun/v3"
)

func TestReservationToken(t *testing.T) { //nolint:cyclop
	t.Run("NoAlloc", func(t *testing.T) {
		stunMsg := &stun.Message{}
		tok := make([]byte, 8)
		if wasAllocs(func() {
			// On stack.
			tk := ReservationToken(tok)
			tk.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}

		tk := make(ReservationToken, 8)
		if wasAllocs(func() {
			// On heap.
			tk.AddTo(stunMsg) //nolint
			stunMsg.Reset()
		}) {
			t.Error("Unexpected allocations")
		}
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		tk := make(ReservationToken, 8)
		tk[2] = 33
		tk[7] = 1
		if err := tk.AddTo(stunMsg); err != nil {
			t.Error(err)
		}
		stunMsg.WriteHeader()
		t.Run("HandleErr", func(t *testing.T) {
			badTk := ReservationToken{34, 45}
			if !stun.IsAttrSizeInvalid(badTk.AddTo(stunMsg)) {
				t.Error("IsAttrSizeInvalid should be true")
			}
		})
		t.Run("GetFrom", func(t *testing.T) {
			decoded := new(stun.Message)
			if _, err := decoded.Write(stunMsg.Raw); err != nil {
				t.Fatal("failed to decode message:", err)
			}
			var tok ReservationToken
			if err := tok.GetFrom(decoded); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(tok, tk) {
				t.Errorf("Decoded %v, expected %v", tok, tk)
			}
			if wasAllocs(func() {
				tok.GetFrom(decoded) //nolint
			}) {
				t.Error("Unexpected allocations")
			}
			t.Run("HandleErr", func(t *testing.T) {
				m := new(stun.Message)
				var handle ReservationToken
				if err := handle.GetFrom(m); !errors.Is(err, stun.ErrAttributeNotFound) {
					t.Errorf("%v should be not found", err)
				}
				m.Add(stun.AttrReservationToken, []byte{1, 2, 3})
				if !stun.IsAttrSizeInvalid(handle.GetFrom(m)) {
					t.Error("IsAttrSizeInvalid should be true")
				}
			})
		})
	})
}
