// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// malformedChannelDataFrame builds a ChannelData frame with a valid channel
// number (0x4000) and a length field of 0xFFFC. Adding the 4-byte header
// overflows uint16 arithmetic, which used to make the computed frame size
// wrap to 0.
func malformedChannelDataFrame() []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint16(buf[0:2], uint16(MinChannelNumber))
	binary.BigEndian.PutUint16(buf[2:4], 0xFFFC)

	return buf
}

// eofConn is a net.Conn whose Read always reports EOF. It lets ReadFrom's
// network-read path return an error instead of blocking.
type eofConn struct{}

func (*eofConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (*eofConn) Write(b []byte) (int, error)      { return len(b), nil }
func (*eofConn) Close() error                     { return nil }
func (*eofConn) LocalAddr() net.Addr              { return nil }
func (*eofConn) RemoteAddr() net.Addr             { return nil }
func (*eofConn) SetDeadline(time.Time) error      { return nil }
func (*eofConn) SetReadDeadline(time.Time) error  { return nil }
func (*eofConn) SetWriteDeadline(time.Time) error { return nil }

// TestOverflowRepro_ConsumeFrame demonstrates the bug at the parser level.
// Before the fix, consumeSingleTURNFrame returned (0, nil) for the malformed
// frame, i.e. "a valid frame of size 0".
func TestOverflowRepro_ConsumeFrame(t *testing.T) {
	n, err := consumeSingleTURNFrame(malformedChannelDataFrame())
	t.Logf("consumeSingleTURNFrame -> n=%d err=%v", n, err)
	assert.ErrorIs(t, err, errIncompleteTURNFrame,
		"an incomplete/oversized frame must return errIncompleteTURNFrame (bug: returned nil)")
}

// TestOverflowRepro_ReadFromDoesNotConsume simulates the server readLoop: a
// zero-size frame with nil error means ReadFrom returns without consuming
// buff, so the caller loops forever at 100% CPU.
func TestOverflowRepro_ReadFromDoesNotConsume(t *testing.T) {
	stunConn := NewSTUNConn(&eofConn{})
	stunConn.buff = malformedChannelDataFrame()

	payload := make([]byte, 1600)
	for range 64 { // a few iterations of what readLoop does
		n, _, err := stunConn.ReadFrom(payload)
		if err != nil {
			// Fixed behavior: instead of returning (0, nil), the connection
			// either blocks waiting for more data or surfaces an error.
			return
		}
		assert.NotEqualf(t, 0, n, "ReadFrom returned a zero-size frame without error (infinite loop)")
		if len(stunConn.buff) == 0 {
			assert.FailNow(t, "ReadFrom consumed the frame without advancing")
		}
	}
	assert.FailNow(t, "ReadFrom returned (0, nil) 64 times in a row: confirmed infinite loop")
}

// maxChannelDataFrame builds a complete ChannelData frame with a declared
// length of 0xFFFC: 65,536 bytes including the 4-byte header.
func maxChannelDataFrame() []byte {
	buf := make([]byte, 65536)
	binary.BigEndian.PutUint16(buf[0:2], uint16(MinChannelNumber))
	binary.BigEndian.PutUint16(buf[2:4], 0xFFFC)

	return buf
}

// TestReadFromOversizedFrameReportsCopiedBytes locks the net.PacketConn
// contract: ReadFrom must never report more bytes than were copied into the
// caller's buffer, even when a complete frame is larger than the buffer.
// Before the fix, ReadFrom returned the full 65,536-byte frame size for a
// 65,535-byte buffer, and callers slicing their buffer by n panicked.
func TestReadFromOversizedFrameReportsCopiedBytes(t *testing.T) {
	stunConn := NewSTUNConn(&eofConn{})
	stunConn.buff = maxChannelDataFrame()

	payload := make([]byte, 65535) // what Client.Listen allocates
	n, _, err := stunConn.ReadFrom(payload)
	assert.NoError(t, err)
	assert.Equal(t, len(payload), n, "must not report more bytes than were copied")
	assert.Empty(t, stunConn.buff, "the whole frame must be consumed to keep framing aligned")
}

// TestReadFromContinuesAfterOversizedFrame verifies that a truncated frame
// does not corrupt the stream: the next frame is still parsed correctly.
func TestReadFromContinuesAfterOversizedFrame(t *testing.T) {
	// 5 data bytes pad to a 12-byte frame, above the 9-byte minimum that
	// consumeSingleTURNFrame needs to tell ChannelData apart from STUN.
	next := &ChannelData{Data: []byte{1, 2, 3, 4, 5}, Number: MinChannelNumber}
	next.Encode()

	stunConn := NewSTUNConn(&eofConn{})
	stunConn.buff = append(maxChannelDataFrame(), next.Raw...)

	payload := make([]byte, 65535)
	n, _, err := stunConn.ReadFrom(payload)
	assert.NoError(t, err)
	assert.Equal(t, len(payload), n)

	small := make([]byte, 12)
	n, _, err = stunConn.ReadFrom(small)
	assert.NoError(t, err)
	assert.Equal(t, len(next.Raw), n, "the frame after a truncated one must parse cleanly")
}
