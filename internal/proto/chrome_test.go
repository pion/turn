// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

func TestChromeAllocRequest(t *testing.T) {
	var (
		r = bytes.NewReader(loadData(t, "01_chromeallocreq.hex"))
		s = bufio.NewScanner(r)

		data     [][]byte
		messages []*stun.Message
	)
	// Decoding hex data into binary.
	for s.Scan() {
		b, err := hex.DecodeString(s.Text())
		assert.NoError(t, err)
		data = append(data, b)
	}
	// All hex streams decoded to raw binary format and stored in data slice.
	// Decoding packets to messages.
	for i, packet := range data {
		m := new(stun.Message)
		_, err := m.Write(packet)
		assert.NoErrorf(t, err, "Packet %d: %v", i, err)
		messages = append(messages, m)
	}
	assert.Equal(t, 4, len(messages), "unexpected number of messages")
}
