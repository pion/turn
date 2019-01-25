package ipnet

import (
	"net"
	"time"

	"golang.org/x/net/ipv4"
)

type ipv4PacketConn struct {
	conn *ipv4.PacketConn
}

func newIPv4PacketConn(c net.PacketConn) (*ipv4PacketConn, error) {
	conn := ipv4.NewPacketConn(c)

	if err := setControlMessage(conn); err != nil {
		return nil, err
	}

	return &ipv4PacketConn{
		conn: conn,
	}, nil
}

func (c *ipv4PacketConn) ReadFromCM(b []byte) (int, *ControlMessage, net.Addr, error) {
	n, ipcm, src, err := c.conn.ReadFrom(b)
	if err != nil {
		return 0, nil, nil, err
	}

	cm := createControlMessage(c.conn, ipcm)

	return n, cm, src, nil
}

func (c *ipv4PacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, _, src, err := c.conn.ReadFrom(p)

	return n, src, err
}

func (c *ipv4PacketConn) WriteTo(b []byte, dst net.Addr) (int, error) {
	return c.conn.WriteTo(b, nil, dst)
}

func (c *ipv4PacketConn) Close() error {
	return c.conn.Close()
}

func (c *ipv4PacketConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()

}

func (c *ipv4PacketConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)

}

func (c *ipv4PacketConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)

}

func (c *ipv4PacketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)

}
