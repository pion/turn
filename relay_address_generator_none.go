// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"fmt"
	"net"
	"strconv"

	"github.com/pion/transport/v3"
	"github.com/pion/transport/v3/stdnet"
)

// RelayAddressGeneratorNone returns the listener with no modifications.
type RelayAddressGeneratorNone struct {
	// Address is passed to Listen/ListenPacket when creating the Relay
	Address string

	Net transport.Net
}

// Validate is called on server startup and confirms the RelayAddressGenerator is properly configured.
func (r *RelayAddressGeneratorNone) Validate() error {
	if r.Net == nil {
		var err error
		r.Net, err = stdnet.NewNet()
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	if r.Address == "" {
		return errListeningAddressInvalid
	}

	return nil
}

// AllocatePacketConn generates a new PacketConn to receive traffic on and the IP/Port
// to populate the allocation response with.
func (r *RelayAddressGeneratorNone) AllocatePacketConn(network string, requestedPort int) (
	net.PacketConn,
	net.Addr,
	error,
) {
	conn, err := r.Net.ListenPacket(network, r.Address+":"+strconv.Itoa(requestedPort)) // nolint: noctx
	if err != nil {
		return nil, nil, err
	}

	return conn, conn.LocalAddr(), nil
}

// AllocateListener generates a new Listener to receive traffic on and the IP/Port
// to populate the allocation response with.
func (r *RelayAddressGeneratorNone) AllocateListener(network string, requestedPort int) (net.Listener, net.Addr, error) { // nolint: lll
	// AllocateListener can be called independently of Validate (e.g. in tests),
	// so ensure we're initialized to avoid nil dereferences.
	if r.Net == nil || r.Address == "" {
		if err := r.Validate(); err != nil {
			return nil, nil, err
		}
	}

	tcpAddr, err := r.Net.ResolveTCPAddr(network, r.Address+":"+strconv.Itoa(requestedPort))
	if err != nil {
		return nil, nil, err
	}

	ln, err := r.Net.ListenTCP(network, tcpAddr) // nolint: noctx
	if err != nil {
		return nil, nil, err
	}

	return ln, ln.Addr(), nil
}
