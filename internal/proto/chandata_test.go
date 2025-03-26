// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelData_Encode(t *testing.T) {
	chanData := &ChannelData{
		Data:   []byte{1, 2, 3, 4},
		Number: MinChannelNumber + 1,
	}
	chanData.Encode()
	b := &ChannelData{}
	b.Raw = append(b.Raw, chanData.Raw...)
	assert.NoError(t, b.Decode())
	assert.True(t, b.Equal(chanData))
	assert.True(t, IsChannelData(b.Raw) && IsChannelData(chanData.Raw))
}

func TestChannelData_Equal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		a, b  *ChannelData
		value bool
	}{
		{
			name:  "nil",
			value: true,
		},
		{
			name: "nil to non-nil",
			b:    &ChannelData{},
		},
		{
			name: "equal",
			b: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3},
			},
			a: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3},
			},
			value: true,
		},
		{
			name: "number",
			b: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3},
			},
			a: &ChannelData{
				Number: MinChannelNumber + 1,
				Data:   []byte{1, 2, 3},
			},
		},
		{
			name: "length",
			b: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3},
			},
			a: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3, 4},
			},
		},
		{
			name: "data",
			b: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 3},
			},
			a: &ChannelData{
				Number: MinChannelNumber,
				Data:   []byte{1, 2, 2},
			},
		},
	} {
		assert.Equal(t, tc.value, tc.a.Equal(tc.b))
	}
}

func TestChannelData_Decode(t *testing.T) {
	for _, tc := range []struct {
		name string
		buf  []byte
		err  error
	}{
		{
			name: "nil",
			err:  io.ErrUnexpectedEOF,
		},
		{
			name: "small",
			buf:  []byte{1, 2, 3},
			err:  io.ErrUnexpectedEOF,
		},
		{
			name: "zeroes",
			buf:  []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			err:  ErrInvalidChannelNumber,
		},
		{
			name: "bad chan number",
			buf:  []byte{63, 255, 0, 0, 0, 4, 0, 0, 1, 2, 3, 4},
			err:  ErrInvalidChannelNumber,
		},
		{
			name: "bad length",
			buf:  []byte{0x40, 0x40, 0x02, 0x23, 0x16, 0, 0, 0, 0, 0, 0, 0},
			err:  ErrBadChannelDataLength,
		},
	} {
		m := &ChannelData{
			Raw: tc.buf,
		}
		assert.ErrorIs(t, m.Decode(), tc.err)
	}
}

func TestChannelData_Reset(t *testing.T) {
	d := &ChannelData{
		Data:   []byte{1, 2, 3, 4},
		Number: MinChannelNumber + 1,
	}
	d.Encode()
	buf := make([]byte, len(d.Raw))
	copy(buf, d.Raw)
	d.Reset()
	d.Raw = buf
	assert.NoError(t, d.Decode())
}

func TestIsChannelData(t *testing.T) {
	for _, tc := range []struct {
		name  string
		buf   []byte
		value bool
	}{
		{
			name: "nil",
		},
		{
			name: "small",
			buf:  []byte{1, 2, 3, 4},
		},
		{
			name: "zeroes",
			buf:  []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
	} {
		assert.Equal(t, tc.value, IsChannelData(tc.buf))
	}
}

func BenchmarkIsChannelData(b *testing.B) {
	buf := []byte{64, 0, 0, 0, 0, 4, 0, 0, 1, 2, 3}
	b.ReportAllocs()
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		IsChannelData(buf)
	}
}

func BenchmarkChannelData_Encode(b *testing.B) {
	d := &ChannelData{
		Data:   []byte{1, 2, 3, 4},
		Number: MinChannelNumber + 1,
	}
	b.ReportAllocs()
	b.SetBytes(4 + channelDataHeaderSize)
	for i := 0; i < b.N; i++ {
		d.Encode()
	}
}

func BenchmarkChannelData_Decode(b *testing.B) {
	d := &ChannelData{
		Data:   []byte{1, 2, 3, 4},
		Number: MinChannelNumber + 1,
	}
	d.Encode()
	buf := make([]byte, len(d.Raw))
	copy(buf, d.Raw)
	b.ReportAllocs()
	b.SetBytes(4 + channelDataHeaderSize)
	for i := 0; i < b.N; i++ {
		d.Reset()
		d.Raw = buf
		assert.NoError(b, d.Decode())
	}
}

func TestChromeChannelData(t *testing.T) {
	var (
		r = bytes.NewReader(loadData(t, "02_chandata.hex"))
		s = bufio.NewScanner(r)

		data     [][]byte
		messages []*ChannelData
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
		chanData := new(ChannelData)
		chanData.Raw = packet
		assert.NoError(t, chanData.Decode(), "Packet %d errored", i)

		encoded := &ChannelData{
			Data:   chanData.Data,
			Number: chanData.Number,
		}
		encoded.Encode()
		decoded := new(ChannelData)
		decoded.Raw = encoded.Raw
		assert.NoError(t, decoded.Decode())
		assert.True(t, decoded.Equal(chanData))

		messages = append(messages, chanData)
	}
	assert.Equal(t, 2, len(messages), "unexpected number of messages")
}
