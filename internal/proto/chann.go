// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"errors"
	"strconv"

	"github.com/pion/stun/v4"
)

// ChannelNumber represents CHANNEL-NUMBER attribute.
//
// The CHANNEL-NUMBER attribute contains the number of the channel.
//
// RFC 5766 Section 14.1.
type ChannelNumber uint16 // Encoded as uint16

func (n ChannelNumber) String() string { return strconv.Itoa(int(n)) }

// 16 bits of uint + 16 bits of RFFU = 0.
const channelNumberSize = 4

// AddTo adds CHANNEL-NUMBER to message.
func (n ChannelNumber) AddTo(m *stun.Message) error {
	v := make([]byte, channelNumberSize)
	binary.BigEndian.PutUint16(v[:2], uint16(n))
	// v[2:4] are zeroes (RFFU = 0)
	m.Add(stun.AttrChannelNumber, v)

	return nil
}

// GetFrom decodes CHANNEL-NUMBER from message.
func (n *ChannelNumber) GetFrom(m *stun.Message) error {
	v, err := m.Get(stun.AttrChannelNumber)
	if err != nil {
		return err
	}
	if err = stun.CheckSize(stun.AttrChannelNumber, len(v), channelNumberSize); err != nil {
		return err
	}
	_ = v[channelNumberSize-1] // Asserting length
	*n = ChannelNumber(binary.BigEndian.Uint16(v[:2]))
	// v[2:4] is RFFU and equals to 0.
	return nil
}

// See https://tools.ietf.org/html/rfc5766#section-11:
//
// 0x4000 through 0x7FFF: These values are the allowed channel
// numbers (16,383 possible values).
//
// and https://tools.ietf.org/html/rfc8656/#section-12:
//
// 0x4000 through 0x4FFF: (4096 possible values).
const (
	MinChannelNumber        ChannelNumber = 0x4000
	MaxChannelNumberRFC5766 ChannelNumber = 0x7FFF
	MaxChannelNumberRFC8656 ChannelNumber = 0x4FFF
)

type ChannelNumberRange struct {
	Min                     ChannelNumber
	Max                     ChannelNumber
	ErrInvalidChannelNumber error
}

// ErrInvalidChannelNumberRFC5766 means that channel number is not valid as by RFC 5766 Section 11.
var ErrInvalidChannelNumberRFC5766 = errors.New("channel number not in [0x4000, 0x7FFF]")

// ErrInvalidChannelNumberRFC8656 means that channel number is not valid as by RFC 8656 Section 12.
var ErrInvalidChannelNumberRFC8656 = errors.New("channel number not in [0x4000, 0x4FFF]")

// ChannelNumberRangeRFC5766 returns the channel number range allowed by RFC 5766 Section 11
// (0x4000-0x7FFF).
func ChannelNumberRangeRFC5766() ChannelNumberRange {
	return ChannelNumberRange{MinChannelNumber, MaxChannelNumberRFC5766, ErrInvalidChannelNumberRFC5766}
}

// ChannelNumberRangeRFC8656 returns the channel number range allowed by RFC 8656 Section 12
// (0x4000-0x4FFF).
func ChannelNumberRangeRFC8656() ChannelNumberRange {
	return ChannelNumberRange{MinChannelNumber, MaxChannelNumberRFC8656, ErrInvalidChannelNumberRFC8656}
}

func (r ChannelNumberRange) Contains(channelNumber ChannelNumber) bool {
	return channelNumber >= r.Min && channelNumber <= r.Max
}
