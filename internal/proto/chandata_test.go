package proto

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

func TestChannelData_Encode(t *testing.T) {
	d := &ChannelData{
		Data:   []byte{1, 2, 3, 4},
		Number: MinChannelNumber + 1,
	}
	d.Encode()
	b := &ChannelData{}
	b.Raw = append(b.Raw, d.Raw...)
	if err := b.Decode(); err != nil {
		t.Error(err)
	}
	if !b.Equal(d) {
		t.Error("not equal")
	}
	if !IsChannelData(b.Raw) || !IsChannelData(d.Raw) {
		t.Error("unexpected IsChannelData")
	}
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
		if v := tc.a.Equal(tc.b); v != tc.value {
			t.Errorf("unexpected: (%s) %v != %v", tc.name, tc.value, v)
		}
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
		if err := m.Decode(); !errors.Is(err, tc.err) {
			t.Errorf("unexpected: (%s) %v != %v", tc.name, tc.err, err)
		}
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
	if err := d.Decode(); err != nil {
		t.Fatal(err)
	}
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
		if v := IsChannelData(tc.buf); v != tc.value {
			t.Errorf("unexpected: (%s) %v != %v", tc.name, tc.value, v)
		}
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
		if err := d.Decode(); err != nil {
			b.Error(err)
		}
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
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, b)
	}
	// All hex streams decoded to raw binary format and stored in data slice.
	// Decoding packets to messages.
	for i, packet := range data {
		m := new(ChannelData)
		m.Raw = packet
		if err := m.Decode(); err != nil {
			t.Errorf("Packet %d: %v", i, err)
		}
		encoded := &ChannelData{
			Data:   m.Data,
			Number: m.Number,
		}
		encoded.Encode()
		decoded := new(ChannelData)
		decoded.Raw = encoded.Raw
		if err := decoded.Decode(); err != nil {
			t.Error(err)
		}
		if !decoded.Equal(m) {
			t.Error("should be equal")
		}

		messages = append(messages, m)
	}
	if len(messages) != 2 {
		t.Error("unexpected message slice list")
	}
}
