// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBase36(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty input",
			input:    []byte{},
			expected: "",
		},
		{
			name:     "zero byte",
			input:    []byte{0},
			expected: "0",
		},
		{
			name:     "single byte - 1",
			input:    []byte{1},
			expected: "1",
		},
		{
			name:     "single byte - 35",
			input:    []byte{35},
			expected: "Z",
		},
		{
			name:     "single byte - 36",
			input:    []byte{36},
			expected: "10",
		},
		{
			name:     "single byte - 255",
			input:    []byte{255},
			expected: "73",
		},
		{
			name:     "multiple bytes - hello",
			input:    []byte("hello"),
			expected: "5PZCSZU7",
		},
		{
			name:     "multiple bytes - test",
			input:    []byte("test"),
			expected: "WANEK4",
		},
		{
			name:     "complex text",
			input:    []byte("long_complexTeXT wITH_space"),
			expected: "6XMY2Y5EIZEF867E5LXYHH2OVVURC1A852VPOAZP0L",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			encoded := encodeBase36(tt.input)
			assert.Equal(t, tt.expected, encoded)
			decoded := decodeBase36(encoded)
			assert.Equal(t, tt.input, decoded)
			decoded = decodeBase36(strings.ToLower(encoded))
			assert.Equal(t, tt.input, decoded)
		})
	}
}
