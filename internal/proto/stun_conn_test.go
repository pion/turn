// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package proto

import (
	"net"
	"testing"
	"time"

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

		n, addr, err := stunConn.ReadFrom(nil)
		assert.Equal(t, n, 8)
		assert.Nil(t, addr)
		assert.NoError(t, err)
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
