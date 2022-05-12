package turn

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/kannonski/turn/v2/internal/proto"
	"github.com/pion/stun"
)

var errInvalidTURNFrame = errors.New("data is not a valid TURN frame, no STUN or ChannelData found")
var errIncompleteTURNFrame = errors.New("data contains incomplete STUN or TURN frame")

// STUNConn wraps a net.Conn and implements
// net.PacketConn by being STUN aware and
// packetizing the stream
type STUNConn struct {
	nextConn net.Conn
	l        sync.RWMutex
	buff     []byte
}

const (
	stunHeaderSize = 20

	channelDataLengthSize = 2
	channelDataNumberSize = channelDataLengthSize
	channelDataHeaderSize = channelDataLengthSize + channelDataNumberSize
	channelDataPadding    = 4
)

// Given a buffer give the last offset of the TURN frame
// If the buffer isn't a valid STUN or ChannelData packet
// or the length doesn't match return false
func consumeSingleTURNFrame(p []byte) (int, error) {
	// Too short to determine if ChannelData or STUN
	if len(p) < 9 {
		return 0, errIncompleteTURNFrame
	}

	var datagramSize uint16
	if stun.IsMessage(p) {
		datagramSize = binary.BigEndian.Uint16(p[2:4]) + stunHeaderSize
	} else if num := binary.BigEndian.Uint16(p[0:4]); proto.ChannelNumber(num).Valid() {
		datagramSize = binary.BigEndian.Uint16(p[channelDataNumberSize:channelDataHeaderSize])
		if paddingOverflow := (datagramSize + channelDataPadding) % channelDataPadding; paddingOverflow != 0 {
			datagramSize = (datagramSize + channelDataPadding) - paddingOverflow
		}
		datagramSize += channelDataHeaderSize
	} else {
		return 0, errInvalidTURNFrame
	}

	if len(p) < int(datagramSize) {
		return 0, errIncompleteTURNFrame
	}

	return int(datagramSize), nil
}

// ReadFrom implements ReadFrom from net.PacketConn
func (s *STUNConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	// First pass any buffered data from previous reads
	s.l.Lock() // Not using defer because this function is recursive
	n, err = consumeSingleTURNFrame(s.buff)
	if err == errInvalidTURNFrame {
		s.l.Unlock()
		return 0, nil, err
	} else if err == nil {
		copy(p, s.buff[:n])
		s.buff = s.buff[n:]
		s.l.Unlock()
		return n, s.nextConn.RemoteAddr(), nil
	}

	// Then read from the nextConn, appending to our buff
	n, err = s.nextConn.Read(p)
	if err != nil {

		s.l.Unlock()
		return 0, nil, err
	}

	s.buff = append(s.buff, p[:n]...)

	s.l.Unlock()
	return s.ReadFrom(p)
}

// WriteTo implements WriteTo from net.PacketConn
func (s *STUNConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return s.nextConn.Write(p)
}

// Close implements Close from net.PacketConn
func (s *STUNConn) Close() error {
	return s.nextConn.Close()
}

// LocalAddr implements LocalAddr from net.PacketConn
func (s *STUNConn) LocalAddr() net.Addr {
	return s.nextConn.LocalAddr()
}

// SetDeadline implements SetDeadline from net.PacketConn
func (s *STUNConn) SetDeadline(t time.Time) error {
	return s.nextConn.SetDeadline(t)
}

// SetReadDeadline implements SetReadDeadline from net.PacketConn
func (s *STUNConn) SetReadDeadline(t time.Time) error {
	return s.nextConn.SetReadDeadline(t)
}

// SetWriteDeadline implements SetWriteDeadline from net.PacketConn
func (s *STUNConn) SetWriteDeadline(t time.Time) error {
	return s.nextConn.SetWriteDeadline(t)
}

// NewSTUNConn creates a STUNConn
func NewSTUNConn(nextConn net.Conn) *STUNConn {
	return &STUNConn{nextConn: nextConn}
}

// Conn a wrapped net.Conn.
// the STUNConn should not be used after a call to Conn
func (s *STUNConn) Conn() net.Conn {
	s.l.Lock() // force no more reads to race
	return &wrappedConn{
		s.nextConn,
		io.MultiReader(bytes.NewReader(s.buff), s.nextConn),
	}
}

type wrappedConn struct {
	net.Conn
	r io.Reader
}

func (w *wrappedConn) Read(p []byte) (n int, err error) {
	return w.r.Read(p)
}
