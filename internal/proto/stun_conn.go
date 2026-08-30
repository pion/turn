// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"errors"
	"net"
	"time"

	"github.com/pion/stun/v3"
)

var (
	errInvalidTURNFrame    = errors.New("data is not a valid TURN frame, no STUN or ChannelData found")
	errIncompleteTURNFrame = errors.New("data contains incomplete STUN or TURN frame")
)

// STUNConn wraps a net.Conn and implements
// net.PacketConn by being STUN aware and
// packetizing the stream.
type STUNConn struct {
	nextConn net.Conn
	buff     []byte
}

const (
	stunHeaderSize     = 20
	channelDataPadding = 4
)

// Given a buffer give the last offset of the TURN frame
// If the buffer isn't a valid STUN or ChannelData packet,
// or the length doesn't match return false.
func consumeSingleTURNFrame(b []byte) (int, error) {
	// Too short to determine if ChannelData or STUN
	if len(b) < 9 {
		return 0, errIncompleteTURNFrame
	}

	// Use uint32 (not uint16) for the frame size: ChannelData length fields
	// near the uint16 maximum (>= 0xFFFC) plus the 4-byte header overflow
	// uint16 arithmetic and wrap to 0, making ReadFrom report a zero-size
	// frame without consuming any buffered data (an infinite loop in the
	// read loop).
	var datagramSize uint32
	switch {
	case stun.IsMessage(b):
		datagramSize = uint32(binary.BigEndian.Uint16(b[2:4])) + stunHeaderSize
	case ChannelNumber(binary.BigEndian.Uint16(b[0:2])).Valid():
		dataLen := uint32(binary.BigEndian.Uint16(b[channelDataNumberSize:channelDataHeaderSize]))
		// Round up to the nearest multiple of the 4-byte padding.
		paddedDataLen := (dataLen + channelDataPadding - 1) &^ (channelDataPadding - 1)
		datagramSize = paddedDataLen + channelDataHeaderSize
	case len(b) < stunHeaderSize:
		return 0, errIncompleteTURNFrame
	default:
		return 0, errInvalidTURNFrame
	}

	if uint64(len(b)) < uint64(datagramSize) {
		return 0, errIncompleteTURNFrame
	}

	return int(datagramSize), nil //nolint:gosec // max frame size is 65540, always fits in int
}

// ReadFrom implements ReadFrom from net.PacketConn.
func (s *STUNConn) ReadFrom(payload []byte) (n int, addr net.Addr, err error) {
	// First pass any buffered data from previous reads
	n, err = consumeSingleTURNFrame(s.buff)
	if errors.Is(err, errInvalidTURNFrame) {
		return 0, nil, err
	} else if err == nil {
		frameSize := n
		// A complete frame can be larger than the caller's buffer (the
		// maximum ChannelData frame is 65,540 bytes). Consume the whole
		// frame from the stream to keep the framing aligned, but report
		// only the bytes actually copied: net.PacketConn forbids
		// n > len(payload).
		if n > len(payload) {
			n = len(payload)
		}
		copy(payload, s.buff[:n])
		s.buff = s.buff[frameSize:]

		return n, s.nextConn.RemoteAddr(), nil
	}

	// Then read from the nextConn, appending to our buff
	n, err = s.nextConn.Read(payload)
	if err != nil {
		return 0, nil, err
	}

	s.buff = append(s.buff, append([]byte{}, payload[:n]...)...)

	return s.ReadFrom(payload)
}

// WriteTo implements WriteTo from net.PacketConn.
func (s *STUNConn) WriteTo(payload []byte, _ net.Addr) (n int, err error) {
	return s.nextConn.Write(payload)
}

// Close implements Close from net.PacketConn.
func (s *STUNConn) Close() error {
	return s.nextConn.Close()
}

// LocalAddr implements LocalAddr from net.PacketConn.
func (s *STUNConn) LocalAddr() net.Addr {
	return s.nextConn.LocalAddr()
}

// SetDeadline implements SetDeadline from net.PacketConn.
func (s *STUNConn) SetDeadline(t time.Time) error {
	return s.nextConn.SetDeadline(t)
}

// SetReadDeadline implements SetReadDeadline from net.PacketConn.
func (s *STUNConn) SetReadDeadline(t time.Time) error {
	return s.nextConn.SetReadDeadline(t)
}

// SetWriteDeadline implements SetWriteDeadline from net.PacketConn.
func (s *STUNConn) SetWriteDeadline(t time.Time) error {
	return s.nextConn.SetWriteDeadline(t)
}

// Conn returns the net.Conn used for this STUNConn.
func (s *STUNConn) Conn() net.Conn {
	return s.nextConn
}

// NewSTUNConn creates a STUNConn.
func NewSTUNConn(nextConn net.Conn) *STUNConn {
	return &STUNConn{nextConn: nextConn}
}
