package turn

import (
	"context"
	"net"
	"strconv"
	"syscall"

	"github.com/pion/transport/vnet"
	"golang.org/x/sys/unix"
)

// RelayAddressGeneratorNone returns the listener with no modifications
type RelayAddressGeneratorNone struct {
	// Address is passed to Listen/ListenPacket when creating the Relay
	Address string

	Net *vnet.Net
}

// Validate is caled on server startup and confirms the RelayAddressGenerator is properly configured
func (r *RelayAddressGeneratorNone) Validate() error {
	if r.Net == nil {
		r.Net = vnet.NewNet(nil)
	}

	switch {
	case r.Address == "":
		return errListeningAddressInvalid
	default:
		return nil
	}
}

// AllocatePacketConn generates a new PacketConn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorNone) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	conn, err := r.Net.ListenPacket(network, net.JoinHostPort(r.Address, strconv.Itoa(requestedPort)))
	if err != nil {
		return nil, nil, err
	}

	return conn, conn.LocalAddr(), nil
}

// AllocateConn generates a new Conn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorNone) AllocateConn(network string, requestedPort int) (net.Listener, net.Addr, error) {
	// TODO: switch to vnet, make multiplatform
	lc := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var err error
			c.Control(func(fd uintptr) {
				err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR|unix.SO_REUSEPORT, 1)
			})
			return err
		},
	}
	listener, err := lc.Listen(context.Background(), network, net.JoinHostPort(r.Address, strconv.Itoa(requestedPort)))
	if err != nil {
		return nil, nil, err
	}

	return listener, listener.Addr(), nil
}
