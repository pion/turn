// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// ChannelData represents The ChannelData Message.
//
// See RFC 5766 Section 11.4.
type ChannelData struct {
	Data   []byte // Can be sub slice of Raw
	Length int    // Ignored while encoding, len(Data) is used
	Number ChannelNumber
	Raw    []byte
}

// Equal returns true if compareTo == c.
func (c *ChannelData) Equal(compareTo *ChannelData) bool {
	if c == nil && compareTo == nil {
		return true
	}
	if c == nil || compareTo == nil {
		return false
	}
	if c.Number != compareTo.Number {
		return false
	}
	if len(c.Data) != len(compareTo.Data) {
		return false
	}

	return bytes.Equal(c.Data, compareTo.Data)
}

// Grow ensures that internal buffer will fit v more bytes and
// increases it capacity if necessary.
//
// Similar to stun.Message.grow method.
func (c *ChannelData) grow(v int) {
	n := len(c.Raw) + v
	for cap(c.Raw) < n {
		c.Raw = append(c.Raw, 0)
	}
	c.Raw = c.Raw[:n]
}

// Reset resets Length, Data and Raw length.
func (c *ChannelData) Reset() {
	c.Raw = c.Raw[:0]
	c.Length = 0
	c.Data = c.Data[:0]
}

// Encode encodes ChannelData Message to Raw.
func (c *ChannelData) Encode() {
	c.Raw = c.Raw[:0]
	c.WriteHeader()
	c.Raw = append(c.Raw, c.Data...)
	padded := nearestPaddedValueLength(len(c.Raw))
	if bytesToAdd := padded - len(c.Raw); bytesToAdd > 0 {
		for range bytesToAdd {
			c.Raw = append(c.Raw, 0)
		}
	}
}

const padding = 4

func nearestPaddedValueLength(l int) int {
	n := padding * (l / padding)
	if n < l {
		n += padding
	}

	return n
}

// WriteHeader writes channel number and length.
func (c *ChannelData) WriteHeader() {
	if len(c.Raw) < channelDataHeaderSize {
		// Making WriteHeader call valid even when c.Raw
		// is nil or len(c.Raw) is less than needed for header.
		c.grow(channelDataHeaderSize)
	}
	// Early bounds check to guarantee safety of writes below.
	_ = c.Raw[:channelDataHeaderSize]
	binary.BigEndian.PutUint16(c.Raw[:channelDataNumberSize], uint16(c.Number))
	binary.BigEndian.PutUint16(c.Raw[channelDataNumberSize:channelDataHeaderSize],
		uint16(len(c.Data)), // nolint:gosec // G115
	)
}

// ErrBadChannelDataLength means that channel data length is not equal
// to actual data length.
var ErrBadChannelDataLength = errors.New("channelData length != len(Data)")

// Decode decodes The ChannelData Message from Raw.
func (c *ChannelData) Decode(channelNumberRange ChannelNumberRange) error {
	buf := c.Raw
	if len(buf) < channelDataHeaderSize {
		return io.ErrUnexpectedEOF
	}
	num := binary.BigEndian.Uint16(buf[:channelDataNumberSize])
	c.Number = ChannelNumber(num)
	l := binary.BigEndian.Uint16(buf[channelDataNumberSize:channelDataHeaderSize])
	c.Data = buf[channelDataHeaderSize:]
	c.Length = int(l)
	if !channelNumberRange.Contains(c.Number) {
		return channelNumberRange.ErrInvalidChannelNumber
	}
	if int(l) < len(c.Data) {
		c.Data = c.Data[:int(l)]
	}
	if int(l) > len(buf[channelDataHeaderSize:]) {
		return ErrBadChannelDataLength
	}

	return nil
}

const (
	channelDataLengthSize = 2
	channelDataNumberSize = channelDataLengthSize
	channelDataHeaderSize = channelDataLengthSize + channelDataNumberSize
)

// IsChannelData returns true if buf is shaped like a ChannelData message: a 4-byte header
// whose length field fits in the buffer. The channel number is deliberately not checked, so
// callers can use any configured channel number range; Decode enforces that range.
// A STUN message also has this shape, so callers must rule out STUN (stun.IsMessage) first.
func IsChannelData(buf []byte) bool {
	if len(buf) < channelDataHeaderSize {
		return false
	}

	return int(binary.BigEndian.Uint16(buf[channelDataNumberSize:channelDataHeaderSize])) <= len(buf[channelDataHeaderSize:])
}
