// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/pion/dtls/v4"
)

// maxDTLSRecordPayload is the DTLS plaintext limit of 2^14 bytes.
// See https://www.rfc-editor.org/rfc/rfc6347#section-4.1
// and https://www.rfc-editor.org/rfc/rfc9147#section-4.4
const maxDTLSRecordPayload = 1 << 14

// DTLSConn adapts a *dtls.Conn to net.PacketConn for TURN over DTLS.
// See https://www.rfc-editor.org/rfc/rfc7350#section-4.6
//
// Each record carries one TURN message, preserving optional ChannelData padding.
// See https://www.rfc-editor.org/rfc/rfc8656#section-12.5
//
// Use it as ClientConfig.Conn; Server wraps accepted DTLS connections automatically.
type DTLSConn struct {
	conn *dtls.Conn

	readMu  sync.Mutex
	readBuf []byte
}

// NewDTLSConn wraps conn for use as a TURN transport.
func NewDTLSConn(conn *dtls.Conn) *DTLSConn {
	return &DTLSConn{conn: conn}
}

// DialDTLS connects to a TURN server, resolving address and completing the handshake within ctx.
// network must be "udp", "udp4" or "udp6".
// The certificate server name defaults to the address host unless overridden by opts.
func DialDTLS(ctx context.Context, network, address string, opts ...dtls.ClientOption) (*DTLSConn, error) {
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, net.UnknownNetworkError(network)
	}

	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// Use a concrete local address to select the allocation address family.
	udpConn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	conn, err := dtls.Client(
		&connectedPacketConn{udpConn},
		udpConn.RemoteAddr(),
		append([]dtls.ClientOption{dtls.WithServerName(host)}, opts...)...,
	)
	if err != nil {
		_ = udpConn.Close()

		return nil, err
	}

	if err := conn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return NewDTLSConn(conn), nil
}

// connectedPacketConn adapts a connected datagram socket to net.PacketConn,
// as WriteTo fails on connected sockets.
type connectedPacketConn struct {
	net.Conn
}

func (c *connectedPacketConn) ReadFrom(payload []byte) (int, net.Addr, error) {
	n, err := c.Read(payload)

	return n, c.RemoteAddr(), err
}

func (c *connectedPacketConn) WriteTo(payload []byte, _ net.Addr) (int, error) {
	return c.Write(payload)
}

// ReadFrom reads one TURN message, truncating it to fit payload as with UDP.
func (c *DTLSConn) ReadFrom(payload []byte) (int, net.Addr, error) {
	buf := payload
	if len(buf) < maxDTLSRecordPayload {
		// Read a full record before truncating; dtls.Conn drops records that do not fit.
		c.readMu.Lock()
		defer c.readMu.Unlock()

		if c.readBuf == nil {
			c.readBuf = make([]byte, maxDTLSRecordPayload)
		}
		buf = c.readBuf
	}

	n, err := c.conn.Read(buf)
	if err != nil {
		return 0, nil, err
	}

	return copy(payload, buf[:n]), c.conn.RemoteAddr(), nil
}

// WriteTo sends p as a single DTLS record to the remote end of the
// association. addr is ignored.
func (c *DTLSConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	return c.conn.Write(p)
}

// Close closes the DTLS association.
func (c *DTLSConn) Close() error {
	return c.conn.Close()
}

// LocalAddr returns the local network address.
func (c *DTLSConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the address of the remote end of the DTLS association.
func (c *DTLSConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline implements net.PacketConn.
func (c *DTLSConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline implements net.PacketConn.
func (c *DTLSConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.PacketConn.
func (c *DTLSConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// ConnectionState returns the latest DTLS state; ok reports whether state is available.
func (c *DTLSConn) ConnectionState() (state dtls.State, ok bool) {
	return c.conn.ConnectionState()
}

// Conn returns the underlying *dtls.Conn.
func (c *DTLSConn) Conn() *dtls.Conn {
	return c.conn
}
