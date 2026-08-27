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
