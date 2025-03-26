// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestDontFragment(t *testing.T) {
	var dontFrag DontFragment

	t.Run("False", func(t *testing.T) {
		m := new(stun.Message)
		m.WriteHeader()
		assert.False(t, dontFrag.IsSet(m))
	})
	t.Run("AddTo", func(t *testing.T) {
		stunMsg := new(stun.Message)
		assert.NoError(t, dontFrag.AddTo(stunMsg))

		stunMsg.WriteHeader()
		t.Run("IsSet", func(t *testing.T) {
			decoded := new(stun.Message)
			_, err := decoded.Write(stunMsg.Raw)
			assert.NoError(t, err)
			assert.True(t, dontFrag.IsSet(stunMsg))

			allocated := wasAllocs(func() {
				dontFrag.IsSet(stunMsg)
			})
			assert.False(t, allocated)
		})
	})
}
