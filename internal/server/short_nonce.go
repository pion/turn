// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// NonceManager interface that both implementations satisfy.
type NonceManager interface {
	Generate() (string, error)
	Validate(nonce string) error
}

const (
	shortNonceLifetime     = time.Hour // Same as original
	shortNonceKeyLength    = 64        // Same as original
	shortNonceTimestampLen = 4         // 6 bytes for timestamp (minutes) - optimal size
	shortNonceMinHMACLen   = 2         // Minimum HMAC length for security
	shortNonceMaxHMACLen   = 32        // Maximum HMAC length (full SHA256)
	defaultNonceHMACLen    = 12        // Default HMAC length
)

// NewShortNonceHash creates a ShortNonceHash. The hmacLen argument specifies the number of HMAC
// bytes to include (2-32 bytes).  The total nonce size will be 4 + hmacLen bytes, default hmaclen
// is 12 bytes. The 4 bytes timestamp gives about ~8000 years before nonces would start to repeat
// (safe until year 10,135).
func NewShortNonceHash(hmacLen int) (NonceManager, error) {
	if hmacLen == 0 {
		hmacLen = defaultNonceHMACLen
	}

	if hmacLen < shortNonceMinHMACLen || hmacLen > shortNonceMaxHMACLen {
		return nil, errFailedToGenerateNonce
	}

	key := make([]byte, shortNonceKeyLength)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: %w", errFailedToGenerateNonce, err)
	}

	return &ShortNonceHash{
		key:     key,
		hmacLen: hmacLen,
	}, nil
}

// ShortNonceHash is used to create and verify short nonces.
type ShortNonceHash struct {
	key     []byte
	hmacLen int
}

// Generate a short nonce (4 + hmacLen bytes encoded as base36).
func (s *ShortNonceHash) Generate() (string, error) {
	timestampMinutes := time.Now().Unix() / 60

	// Convert to bytes and trim to 4 bytes.  This safely handles the conversion since we know
	// current values fit in 4 bytes until year 10,135.
	timestampBytes8 := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes8, uint64(timestampMinutes)) // nolint:gosec // G115
	timestampBytes := timestampBytes8[4:]

	hash := hmac.New(sha256.New, s.key)
	if _, err := hash.Write(timestampBytes); err != nil {
		return "", fmt.Errorf("%w: %w", errFailedToGenerateNonce, err)
	}
	fullHMAC := hash.Sum(nil)
	truncatedHMAC := fullHMAC[:s.hmacLen]

	totalLen := shortNonceTimestampLen + s.hmacLen
	nonce := make([]byte, totalLen)
	copy(nonce[:shortNonceTimestampLen], timestampBytes)
	copy(nonce[shortNonceTimestampLen:], truncatedHMAC)

	return encodeBase36(nonce), nil
}

// Validate checks that nonce is signed and is not expired.
func (s *ShortNonceHash) Validate(nonce string) error {
	nonceBytes := decodeBase36(nonce)
	if nonceBytes == nil {
		return errInvalidNonce
	}

	expectedLen := shortNonceTimestampLen + s.hmacLen
	if len(nonceBytes) != expectedLen {
		// Pad with leadnign zeros if leading zeros were stripped during encoding/decoding.
		if len(nonceBytes) < expectedLen {
			padded := make([]byte, expectedLen)
			copy(padded[expectedLen-len(nonceBytes):], nonceBytes)
			nonceBytes = padded
		} else {
			return errInvalidNonce
		}
	}

	timestampBytes := nonceBytes[:shortNonceTimestampLen]
	receivedHMAC := nonceBytes[shortNonceTimestampLen:]
	timestampMinutes := int64(binary.BigEndian.Uint32(timestampBytes))

	// Check if nonce is expired (older than 1 hour).
	currentMinutes := time.Now().Unix() / 60
	if currentMinutes < timestampMinutes {
		return errInvalidNonce
	}

	ageMinutes := currentMinutes - timestampMinutes
	if ageMinutes > 60 {
		return errInvalidNonce
	}

	// Recompute HMAC and compare.
	hash := hmac.New(sha256.New, s.key)
	if _, err := hash.Write(timestampBytes); err != nil {
		return fmt.Errorf("%w: %w", errInvalidNonce, err)
	}

	expectedHMAC := hash.Sum(nil)[:s.hmacLen]

	if !hmac.Equal(receivedHMAC, expectedHMAC) {
		return errInvalidNonce
	}

	return nil
}
