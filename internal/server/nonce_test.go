// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNonceHash(t *testing.T) {
	t.Run("generated hashes validate", func(t *testing.T) {
		h, err := NewNonceHash()
		assert.NoError(t, err)
		nonce, err := h.Generate()
		assert.NoError(t, err)
		assert.NoError(t, h.Validate(nonce))
	})
}
