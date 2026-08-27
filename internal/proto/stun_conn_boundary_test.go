// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"testing"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

// oldPaddedSize replicates the original (buggy) uint16 padding math, widened
// to uint32 so its arithmetic never overflows. It serves as the reference
// implementation for exhaustively proving the new padding formula.
func oldPaddedSize(x uint32) uint32 {
	s := x
	if paddingOverflow := (s + channelDataPadding) % channelDataPadding; paddingOverflow != 0 {
		s = (s + channelDataPadding) - paddingOverflow
	}

	return s
}

func newPaddedSize(x uint32) uint32 {
	return (x + channelDataPadding - 1) &^ (channelDataPadding - 1)
}

// TestPaddingFormulaEquivalence exhaustively proves that the new padding
// formula yields identical results to the original one for every possible
// uint16 length field value.
func TestPaddingFormulaEquivalence(t *testing.T) {
	for x := uint32(0); x <= 0xFFFF; x++ {
		assert.Equal(t, oldPaddedSize(x), newPaddedSize(x), "padding formula diverged at %d", x)
	}
}

func frameWithChannelDataLen(length uint16) []byte {
	b := make([]byte, 16)
	binary.BigEndian.PutUint16(b[0:2], uint16(MinChannelNumber))
	binary.BigEndian.PutUint16(b[2:4], length)

	return b
}

// TestOverflowBoundaryScan covers every ChannelData length field near the
// uint16 maximum. Before the fix, all of these wrapped to a zero-size frame
// with a nil error.
func TestOverflowBoundaryScan(t *testing.T) {
	for l := uint32(0xFFF8); l <= 0xFFFF; l++ {
		frame := frameWithChannelDataLen(uint16(l))
		n, err := consumeSingleTURNFrame(frame)
		// 16 available bytes can never hold a complete frame this large.
		assert.ErrorIs(t, err, errIncompleteTURNFrame, "length=%#x: expected incomplete, got n=%d err=%v", l, n, err)
	}
}

// TestSTUNLengthOverflow covers the same overflow class on the STUN branch:
// a declared length of 0xFFFD used to wrap to a 17-byte "frame" (uint16
// arithmetic), silently truncating real STUN messages.
func TestSTUNLengthOverflow(t *testing.T) {
	// 20-byte STUN header with valid magic cookie and a length field near
	// the uint16 maximum.
	b := make([]byte, stunHeaderSize)
	b[0] = 0x00 // message type, 2 high bits must be 0
	b[1] = 0x01 // binding request
	binary.BigEndian.PutUint16(b[2:4], 0xFFFD)
	binary.BigEndian.PutUint32(b[4:8], 0x2112A442) // STUN magic cookie
	assert.True(t, stun.IsMessage(b), "test setup: must look like a STUN message")

	n, err := consumeSingleTURNFrame(b)
	assert.ErrorIs(t, err, errIncompleteTURNFrame, "got n=%d err=%v", n, err)
}

// TestMaxChannelDataFrame ensures the largest legal ChannelData frame
// (length 0xFFFF -> 65540 bytes with header+padding) is computed exactly.
func TestMaxChannelDataFrame(t *testing.T) {
	frame := frameWithChannelDataLen(0xFFFF)
	full := make([]byte, 65540)
	copy(full, frame)

	n, err := consumeSingleTURNFrame(full)
	assert.NoError(t, err)
	assert.Equal(t, 65540, n)
}

// TestZeroLengthChannelData keeps the pre-existing behavior: an empty
// ChannelData payload yields a 4-byte frame.
func TestZeroLengthChannelData(t *testing.T) {
	frame := frameWithChannelDataLen(0x0000)
	n, err := consumeSingleTURNFrame(frame)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
}

// TestNonMultipleLengthChannelData verifies normal padding behavior is
// unchanged (length 5 -> 8 bytes padded + 4 header = 12).
func TestNonMultipleLengthChannelData(t *testing.T) {
	frame := frameWithChannelDataLen(0x0005)
	full := make([]byte, 12)
	copy(full, frame)

	n, err := consumeSingleTURNFrame(full)
	assert.NoError(t, err)
	assert.Equal(t, 12, n)
}
