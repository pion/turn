// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/reuseport"
	"github.com/pion/transport/v4/stdnet"
)

// RelayAddressGeneratorStatic can be used to return static IP address each time a relay is created.
// This can be used when you have a single static IP address that you want to use.
type RelayAddressGeneratorStatic struct {
	// RelayAddress is the IP returned to the user when the relay is created
	RelayAddress net.IP

	// Address is passed to Listen/ListenPacket when creating the Relay
	Address string

	Net transport.Net
}

// Validate is called on server startup and confirms the RelayAddressGenerator is properly configured.
func (r *RelayAddressGeneratorStatic) Validate() error {
	if r.Net == nil {
		var err error
		r.Net, err = stdnet.NewNet()
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	switch {
	case r.RelayAddress == nil:
		return errRelayAddressInvalid
	case r.Address == "":
		return errListeningAddressInvalid
	default:
		return nil
	}
}

// AllocatePacketConn generates a new PacketConn to receive traffic on and the IP/Port
// to populate the allocation response with.
func (r *RelayAddressGeneratorStatic) AllocatePacketConn(
	network string,
	requestedPort int,
) (net.PacketConn, net.Addr, error) {
	conn, err := r.Net.ListenPacket(network, net.JoinHostPort(r.Address, strconv.Itoa(requestedPort))) // nolint: noctx
	if err != nil {
		return nil, nil, err
	}

	// Replace actual listening IP with the user requested one of RelayAddressGeneratorStatic
	relayAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, nil, errNilConn
	}

	relayAddr.IP = r.RelayAddress

	return conn, relayAddr, nil
}

// AllocateListener generates a new Listener to receive traffic on and the IP/Port
// to populate the allocation response with.
func (r *RelayAddressGeneratorStatic) AllocateListener(network string, requestedPort int) (net.Listener, net.Addr, error) { // nolint: lll
	// AllocateListener can be called independently of Validate (e.g. in tests),
	// so ensure we're initialized to avoid nil dereferences.
	if r.Net == nil || r.Address == "" || r.RelayAddress == nil {
		if err := r.Validate(); err != nil {
			return nil, nil, err
		}
	}

	tcpAddr, err := r.Net.ResolveTCPAddr(network, net.JoinHostPort(r.Address, strconv.Itoa(requestedPort)))
	if err != nil {
		return nil, nil, err
	}

	listenConfig := r.Net.CreateListenConfig(&net.ListenConfig{
		// Enable SO_REUSEADDR and SO_REUSEPORT where needed to let multiple connnections
		// bind to the same relay address.
		Control: reuseport.Control,
	})
	ln, err := listenConfig.Listen(context.TODO(), network, tcpAddr.String())
	if err != nil {
		return nil, nil, err
	}

	// Replace actual listening IP with the user requested one of RelayAddressGeneratorStatic
	relayAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		_ = ln.Close()

		return nil, nil, errNilConn
	}

	relayAddr.IP = r.RelayAddress

	return ln, relayAddr, nil
}

// AllocateConn creates a new outgoing TCP connection bound to the relay address to send traffic to a peer.
func (r *RelayAddressGeneratorStatic) AllocateConn(network string, laddr, raddr net.Addr) (net.Conn, error) {
	dialer := r.Net.CreateDialer(&net.Dialer{
		LocalAddr: laddr,
		// Enable SO_REUSEADDR and SO_REUSEPORT where needed to let multiple connnections
		// bind to the same relay address.
		Control: reuseport.Control,
	})

	return dialer.Dial(network, raddr.String())
}
