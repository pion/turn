// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js

package turn

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oversizedFrameConn serves a fixed byte stream followed by EOF, pretending
// to be the TCP connection between a client and a TURN server.
type oversizedFrameConn struct {
	data []byte
	pos  int
	addr net.Addr
}

func (c *oversizedFrameConn) Read(p []byte) (int, error) {
	if c.pos >= len(c.data) {
		return 0, io.EOF
	}
	n := copy(p, c.data[c.pos:])
	c.pos += n

	return n, nil
}

func (c *oversizedFrameConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *oversizedFrameConn) Close() error { return nil }

func (c *oversizedFrameConn) LocalAddr() net.Addr { return c.addr }

func (c *oversizedFrameConn) RemoteAddr() net.Addr { return c.addr }

func (c *oversizedFrameConn) SetDeadline(time.Time) error { return nil }

func (c *oversizedFrameConn) SetReadDeadline(time.Time) error { return nil }

func (c *oversizedFrameConn) SetWriteDeadline(time.Time) error { return nil }

// TestClientListenWithOversizedChannelData reproduces the panic reported for
// the documented NewSTUNConn(net.Conn) client path: a complete ChannelData
// frame of 65,536 bytes (length 0xFFFC) is larger than the 65,535-byte
// buffer allocated by Client.Listen. ReadFrom must not report a frame size
// larger than the buffer, otherwise Listen slices buf[:n] out of range.
func TestClientListenWithOversizedChannelData(t *testing.T) {
	frame := make([]byte, 65536)
	binary.BigEndian.PutUint16(frame[0:2], 0x4000) // valid channel number
	binary.BigEndian.PutUint16(frame[2:4], 0xFFFC) // declared length near uint16 max

	addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 3478}
	client, err := NewClient(&ClientConfig{
		Conn:           NewSTUNConn(&oversizedFrameConn{data: frame, addr: addr}),
		TURNServerAddr: "127.0.0.1:3478",
	})
	require.NoError(t, err)

	// Mirror the Client.Listen read loop (client.go). Before the fix this
	// panics with "slice bounds out of range [:65536] with capacity 65535".
	assert.NotPanics(t, func() {
		buf := make([]byte, maxDataBufferSize)
		for {
			n, from, readErr := client.conn.ReadFrom(buf)
			if readErr != nil {
				return
			}
			// The truncated frame is not valid ChannelData, so it is
			// ignored as application data and the loop keeps going.
			_, handleErr := client.HandleInbound(buf[:n], from)
			if handleErr != nil {
				return
			}
		}
	})
}
