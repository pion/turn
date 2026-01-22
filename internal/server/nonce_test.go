// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"fmt"
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

	t.Run("generated short hashes validate", func(t *testing.T) {
		for i := 1; i <= 8; i++ {
			h, err := NewShortNonceHash(i * 4)
			assert.NoError(t, err, fmt.Sprintf("nonce init at size %d", i*4))
			nonce, err := h.Generate()
			assert.NoError(t, err, fmt.Sprintf("generate at size %d", i*4))
			assert.NoError(t, h.Validate(nonce), fmt.Sprintf("decode at size %d", i*4))
		}
	})
}
