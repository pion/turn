// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/pion/transport/v2"
	"github.com/pion/transport/v2/stdnet"
	"golang.org/x/sys/unix"
)

// RelayAddressGeneratorNone returns the listener with no modifications
type RelayAddressGeneratorNone struct {
	// Address is passed to Listen/ListenPacket when creating the Relay
	Address string

	Net transport.Net
}

// Validate is called on server startup and confirms the RelayAddressGenerator is properly configured
func (r *RelayAddressGeneratorNone) Validate() error {
	if r.Net == nil {
		var err error
		r.Net, err = stdnet.NewNet()
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
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
	conn, err := r.Net.ListenPacket(network, r.Address+":"+strconv.Itoa(requestedPort))
	if err != nil {
		return nil, nil, err
	}

	return conn, conn.LocalAddr(), nil
}

// AllocateListener generates a new Listener to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorNone) AllocateListener(network string, requestedPort int) (net.Listener, net.Addr, error) {
	config := &net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var err error
		c.Control(func(fd uintptr) {
			err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR|unix.SO_REUSEPORT, 1)
		})
		return err
	}}
	listener, err := config.Listen(context.Background(), network, r.Address+":"+strconv.Itoa(requestedPort))
	if err != nil {
		return nil, nil, err
	}

	return listener, listener.Addr(), nil
}

// AllocateConn generates a new Conn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorNone) AllocateConn(string, int) (net.Conn, net.Addr, error) {
	return nil, nil, errTODO
}
