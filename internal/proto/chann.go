// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
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
const (
	MinChannelNumber ChannelNumber = 0x4000
	MaxChannelNumber ChannelNumber = 0x7FFF
)

type ChannelNumberRange struct {
	Min                     ChannelNumber
	Max                     ChannelNumber
	ErrInvalidChannelNumber error
}

func (r ChannelNumberRange) Contains(channelNumber ChannelNumber) bool {
	return channelNumber >= r.Min && channelNumber <= r.Max
}
