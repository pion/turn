// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"math/big"
	"strings"
)

// Base36 alphabet for encoding.
const base36Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

// EncodeBase36 converts bytes to base36 string using big.Int for arbitrary length.
func encodeBase36(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	num := new(big.Int).SetBytes(data)
	if num.Cmp(big.NewInt(0)) == 0 {
		return "0"
	}

	base := big.NewInt(36)
	buf := make([]byte, 0, len(data)*2)
	remainder := new(big.Int)
	for num.Cmp(big.NewInt(0)) > 0 {
		num.DivMod(num, base, remainder)
		buf = append(buf, base36Alphabet[remainder.Int64()])
	}

	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}

// DecodeBase36 converts base36 string back to bytes using big.Int for arbitrary length.
func decodeBase36(encoded string) []byte {
	if encoded == "" {
		return []byte{}
	}

	if encoded == "0" {
		return []byte{0}
	}

	num := big.NewInt(0)
	base := big.NewInt(36)

	for _, char := range strings.ToUpper(encoded) {
		digit := strings.IndexRune(base36Alphabet, char)
		if digit == -1 {
			return nil
		}
		num.Mul(num, base)
		num.Add(num, big.NewInt(int64(digit)))
	}

	return num.Bytes()
}
