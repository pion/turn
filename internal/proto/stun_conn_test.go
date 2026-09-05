// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/stretchr/testify/assert"
)

type mockConn struct {
	didClose, didLocalAddr, didRemoteAddr, didSetWriteDeadline, didSetDeadline, didSetReadDeadline bool
}

func (m *mockConn) Read(b []byte) (n int, err error) { return }

func (m *mockConn) Write(b []byte) (n int, err error) { return }

func (m *mockConn) Close() error {
	m.didClose = true

	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	m.didLocalAddr = true

	return nil
}

func (m *mockConn) RemoteAddr() net.Addr {
	m.didRemoteAddr = true

	return nil
}

func (m *mockConn) SetDeadline(t time.Time) error {
	m.didSetDeadline = true

	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	m.didSetReadDeadline = true

	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	m.didSetWriteDeadline = true

	return nil
}

func TestStunConn(t *testing.T) {
	t.Run("nextConn Called", func(t *testing.T) {
		testConn := &mockConn{}
		stunConn := NewSTUNConn(testConn)

		assert.Nil(t, stunConn.LocalAddr())
		assert.True(t, testConn.didLocalAddr)

		assert.NoError(t, stunConn.Close())
		assert.True(t, testConn.didClose)

		assert.NoError(t, stunConn.SetDeadline(time.Time{}))
		assert.True(t, testConn.didSetDeadline)

		assert.NoError(t, stunConn.SetReadDeadline(time.Time{}))
		assert.True(t, testConn.didSetReadDeadline)

		assert.NoError(t, stunConn.SetWriteDeadline(time.Time{}))
		assert.True(t, testConn.didSetWriteDeadline)
	})

	t.Run("Invalid STUN Frames", func(t *testing.T) {
		testConn := &mockConn{}
		stunConn := NewSTUNConn(testConn)
		stunConn.buff = make([]byte, stunHeaderSize+1)

		n, addr, err := stunConn.ReadFrom(nil)
		assert.Zero(t, n)
		assert.Nil(t, addr)
		assert.Error(t, err, errInvalidTURNFrame)
	})

	t.Run("Invalid ChannelData size", func(t *testing.T) {
		n, err := consumeSingleTURNFrame([]byte{0x40, 0x00, 0x00, 0x12, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
		assert.Equal(t, n, 0)
		assert.Error(t, err, errIncompleteTURNFrame)
	})

	t.Run("Padding", func(t *testing.T) {
		testConn := &mockConn{}
		stunConn := NewSTUNConn(testConn)
		stunConn.buff = []byte{0x40, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

		n, addr, err := stunConn.ReadFrom(make([]byte, 8))
		assert.Equal(t, n, 8)
		assert.Nil(t, addr)
		assert.NoError(t, err)
	})

	t.Run("Frame Larger Than Buffer", func(t *testing.T) {
		// A complete ChannelData frame of 65540 bytes cannot fit in a
		// 1600-byte buffer. ReadFrom must reject it rather than report more
		// bytes than it copied, and it must consume the frame so the next
		// one still parses.
		next := &ChannelData{Data: []byte{1, 2, 3, 4, 5}, Number: MinChannelNumber}
		next.Encode()

		stunConn := NewSTUNConn(&mockConn{})
		stunConn.buff = append(channelDataFrame(0xFFFF, 65540), next.Raw...)

		payload := make([]byte, 1600)
		n, addr, err := stunConn.ReadFrom(payload)
		assert.Zero(t, n)
		assert.Nil(t, addr)
		assert.ErrorIs(t, err, errTURNFrameTooLarge)

		n, _, err = stunConn.ReadFrom(payload)
		assert.NoError(t, err)
		assert.Equal(t, len(next.Raw), n)
	})
}

// channelDataFrame builds a ChannelData frame of total size bytes whose
// declared length field is length.
func channelDataFrame(length uint16, size int) []byte {
	b := make([]byte, size)
	binary.BigEndian.PutUint16(b[0:2], uint16(MinChannelNumber))
	binary.BigEndian.PutUint16(b[2:4], length)

	return b
}

// oldPaddedSize replicates the original uint16 padding math, widened to
// uint32 so its arithmetic never overflows. It is the reference for proving
// the rewritten padding formula is equivalent.
func oldPaddedSize(x uint32) uint32 {
	s := x
	if paddingOverflow := (s + channelDataPadding) % channelDataPadding; paddingOverflow != 0 {
		s = (s + channelDataPadding) - paddingOverflow
	}

	return s
}

// TestPaddingFormulaEquivalence exhaustively proves the rewritten padding
// formula matches the original one for every possible uint16 length.
func TestPaddingFormulaEquivalence(t *testing.T) {
	for x := uint32(0); x <= 0xFFFF; x++ {
		padded := (x + channelDataPadding - 1) &^ (channelDataPadding - 1)
		assert.Equal(t, oldPaddedSize(x), padded, "padding formula diverged at %d", x)
	}
}

// TestConsumeSingleTURNFrameLengthOverflow covers the length values whose
// frame size overflowed uint16 arithmetic and wrapped to a small (or zero)
// size, making ReadFrom return a valid frame that consumed nothing.
func TestConsumeSingleTURNFrameLengthOverflow(t *testing.T) {
	t.Run("ChannelData", func(t *testing.T) {
		for l := uint32(0xFFF8); l <= 0xFFFF; l++ {
			// 16 available bytes can never hold a frame this large.
			n, err := consumeSingleTURNFrame(channelDataFrame(uint16(l), 16)) //nolint:gosec // loop bound is 0xFFFF
			assert.ErrorIs(t, err, errIncompleteTURNFrame, "length=%#x returned n=%d", l, n)
		}
	})

	t.Run("STUN", func(t *testing.T) {
		// A declared length of 0xFFFD used to wrap to a 17-byte frame.
		b := make([]byte, stunHeaderSize)
		b[1] = 0x01 // binding request
		binary.BigEndian.PutUint16(b[2:4], 0xFFFD)
		binary.BigEndian.PutUint32(b[4:8], 0x2112A442) // magic cookie
		assert.True(t, stun.IsMessage(b))

		n, err := consumeSingleTURNFrame(b)
		assert.ErrorIs(t, err, errIncompleteTURNFrame, "returned n=%d", n)
	})

	t.Run("Max Frame", func(t *testing.T) {
		// The largest legal ChannelData frame is computed exactly.
		n, err := consumeSingleTURNFrame(channelDataFrame(0xFFFF, 65540))
		assert.NoError(t, err)
		assert.Equal(t, 65540, n)
	})
}

func TestConsumeSingleTURNFrame(t *testing.T) {
	type testCase struct {
		data []byte
		err  error
	}
	cases := map[string]testCase{
		"channel data": {
			data: []byte{0x40, 0x01, 0x00, 0x08, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			err:  nil,
		},
		"partial data less than channel header": {
			data: []byte{1},
			err:  errIncompleteTURNFrame,
		},
		"partial stun message": {
			data: []byte{0x0, 0x16, 0x02, 0xDC, 0x21, 0x12, 0xA4, 0x42, 0x0, 0x0, 0x0},
			err:  errIncompleteTURNFrame,
		},
		"stun message": {
			data: []byte{
				0x00, 0x16, 0x00, 0x02, 0x21, 0x12, 0xA4, 0x42, 0xf7, 0x43, 0x81,
				0xa3, 0xc9, 0xcd, 0x88, 0x89, 0x70, 0x58, 0xac, 0x73, 0x00, 0x00,
			},
		},
	}

	for name, cs := range cases {
		c := cs
		t.Run(name, func(t *testing.T) {
			n, e := consumeSingleTURNFrame(c.data)
			assert.Equal(t, c.err, e)
			if e == nil {
				assert.Equal(t, len(c.data), n)
			}
		})
	}
}
