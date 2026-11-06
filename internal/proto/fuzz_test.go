// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/pion/stun/v4"
)

type attr interface {
	stun.Getter
	stun.Setter
}

type attrs []struct {
	g attr
	t stun.AttrType
}

func (a attrs) pick(v byte) struct {
	g attr
	t stun.AttrType
} {
	idx := int(v) % len(a)

	return a[idx]
}

func FuzzSetters(f *testing.F) {
	f.Fuzz(func(_ *testing.T, attrType byte, value []byte) {
		var (
			m1 = &stun.Message{
				Raw: make([]byte, 0, 2048),
			}
			m2 = &stun.Message{
				Raw: make([]byte, 0, 2048),
			}
			m3 = &stun.Message{
				Raw: make([]byte, 0, 2048),
			}
		)
		attributes := attrs{
			{new(ChannelNumber), stun.AttrChannelNumber},
			{new(Lifetime), stun.AttrLifetime},
			{new(XORPeerAddress), stun.AttrXORPeerAddress},
			{new(Data), stun.AttrData},
			{new(XORRelayedAddress), stun.AttrXORRelayedAddress},
			{new(EvenPort), stun.AttrEvenPort},
			{new(RequestedTransport), stun.AttrRequestedTransport},
			{new(DontFragment), stun.AttrDontFragment},
			{new(ReservationToken), stun.AttrReservationToken},
			{new(ConnectionID), stun.AttrConnectionID},
			{new(RequestedAddressFamily), stun.AttrRequestedAddressFamily},
		}

		attr := attributes.pick(attrType)

		m1.WriteHeader()
		m1.Add(attr.t, value)
		if err := attr.g.GetFrom(m1); err != nil {
			if errors.Is(err, stun.ErrAttributeNotFound) {
				fmt.Println("unexpected 404") //nolint
				panic(err)                    //nolint
			}

			return
		}

		m2.WriteHeader()
		if err := attr.g.AddTo(m2); err != nil {
			fmt.Println("failed to add attribute to m2") //nolint
			panic(err)                                   //nolint
		}

		m3.WriteHeader()
		v, err := m2.Get(attr.t)
		if err != nil {
			panic(err) //nolint
		}
		m3.Add(attr.t, v)

		if !m2.Equal(m3) {
			fmt.Println(m2, "not equal", m3) //nolint
			panic("not equal")               //nolint
		}
	})
}

func FuzzChannelData(f *testing.F) {
	channelData := &ChannelData{}
	channelNumberRange := ChannelNumberRangeRFC5766() // default number range

	f.Fuzz(func(_ *testing.T, data []byte) {
		channelData.Reset()

		if len(data) > channelDataHeaderSize {
			// Make sure the channel id is in the proper range
			if b := binary.BigEndian.Uint16(data[0:4]); b > 20000 {
				binary.BigEndian.PutUint16(data[0:4], uint16(MinChannelNumber)-1)
			} else if b > 40000 {
				minChannelNumber := uint16(MinChannelNumber)
				channelNumber := uint16(channelNumberRange.Max - MinChannelNumber)
				binary.BigEndian.PutUint16(data[0:4], minChannelNumber+channelNumber%b)
			}
		}

		channelData.Raw = append(channelData.Raw, data...)
		if channelData.Decode(channelNumberRange) != nil {
			return
		}

		channelData.Encode()
		if !channelNumberRange.Contains(channelData.Number) {
			return
		}

		d2 := &ChannelData{}
		d2.Raw = channelData.Raw
		if err := d2.Decode(channelNumberRange); err != nil {
			panic(err) //nolint
		}
	})
}

func FuzzIsChannelData(f *testing.F) {
	f.Fuzz(func(_ *testing.T, data []byte) {
		IsChannelData(data)
	})
}
