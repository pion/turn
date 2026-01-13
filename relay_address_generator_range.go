// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/pion/randutil"
	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/reuseport"
	"github.com/pion/transport/v4/stdnet"
)

// RelayAddressGeneratorPortRange can be used to only allocate connections inside a defined port range.
// Similar to the RelayAddressGeneratorStatic a static ip address can be set.
type RelayAddressGeneratorPortRange struct {
	// RelayAddress is the IP returned to the user when the relay is created
	RelayAddress net.IP

	// MinPort the minimum port to allocate
	MinPort uint16
	// MaxPort the maximum (inclusive) port to allocate
	MaxPort uint16

	// MaxRetries the amount of tries to allocate a random port in the defined range
	MaxRetries int

	// Rand the random source of numbers
	Rand randutil.MathRandomGenerator

	// Address is passed to Listen/ListenPacket when creating the Relay
	Address string

	Net transport.Net
}

// Validate is called on server startup and confirms the RelayAddressGenerator is properly configured.
func (r *RelayAddressGeneratorPortRange) Validate() error {
	if r.Net == nil {
		var err error
		r.Net, err = stdnet.NewNet()
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	if r.Rand == nil {
		r.Rand = randutil.NewMathRandomGenerator()
	}

	if r.MaxRetries == 0 {
		r.MaxRetries = 10
	}

	switch {
	case r.MinPort == 0:
		return errMinPortNotZero
	case r.MaxPort == 0:
		return errMaxPortNotZero
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
func (r *RelayAddressGeneratorPortRange) AllocatePacketConn(
	network string,
	requestedPort int,
) (net.PacketConn, net.Addr, error) {
	if requestedPort != 0 {
		listenAddr := net.JoinHostPort(r.Address, strconv.Itoa(requestedPort))
		conn, err := r.Net.ListenPacket(network, listenAddr) // nolint: noctx
		if err != nil {
			return nil, nil, err
		}

		relayAddr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok {
			return nil, nil, errNilConn
		}

		relayAddr.IP = r.RelayAddress

		return conn, relayAddr, nil
	}

	for try := 0; try < r.MaxRetries; try++ {
		port := r.MinPort + uint16(r.Rand.Intn(int((r.MaxPort+1)-r.MinPort))) // nolint:gosec // G115 false positive
		listenAddr := net.JoinHostPort(r.Address, strconv.Itoa(int(port)))
		conn, err := r.Net.ListenPacket(network, listenAddr) // nolint: noctx
		if err != nil {
			continue
		}

		relayAddr, ok := conn.LocalAddr().(*net.UDPAddr)
		if !ok {
			return nil, nil, errNilConn
		}

		relayAddr.IP = r.RelayAddress

		return conn, relayAddr, nil
	}

	return nil, nil, errMaxRetriesExceeded
}

// AllocateListener generates a new Listener to receive traffic on and the IP/Port
// to populate the allocation response with.
func (r *RelayAddressGeneratorPortRange) AllocateListener( // nolint: cyclop
	network string,
	requestedPort int,
) (net.Listener, net.Addr, error) {
	// AllocateListener can be called independently of Validate (e.g. in tests),
	// so ensure we're initialized to avoid nil dereferences.
	if r.Net == nil || r.Address == "" || r.RelayAddress == nil || r.Rand == nil {
		if err := r.Validate(); err != nil {
			return nil, nil, err
		}
	}

	listenConfig := r.Net.CreateListenConfig(&net.ListenConfig{
		// Enable SO_REUSEADDR and SO_REUSEPORT where needed to let multiple connnections
		// bind to the same relay address.
		Control: reuseport.Control,
	})
	listen := func(port int) (net.Listener, net.Addr, error) {
		listenAddr := net.JoinHostPort(r.Address, strconv.Itoa(port))
		tcpAddr, err := r.Net.ResolveTCPAddr(network, listenAddr)
		if err != nil {
			return nil, nil, err
		}

		ln, err := listenConfig.Listen(context.TODO(), network, tcpAddr.String())
		if err != nil {
			return nil, nil, err
		}

		relayAddr, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			_ = ln.Close()

			return nil, nil, errNilConn
		}

		relayAddr.IP = r.RelayAddress

		return ln, relayAddr, nil
	}

	if requestedPort != 0 {
		return listen(requestedPort)
	}

	for try := 0; try < r.MaxRetries; try++ {
		port := int(r.MinPort + uint16(r.Rand.Intn(int((r.MaxPort+1)-r.MinPort)))) // nolint:gosec // G115 false positive
		ln, addr, err := listen(port)
		if err != nil {
			continue
		}

		return ln, addr, nil
	}

	return nil, nil, errMaxRetriesExceeded
}

// AllocateConn creates a new outgoing TCP connection bound to the relay address to send traffic to a peer.
func (r *RelayAddressGeneratorPortRange) AllocateConn(network string, laddr, raddr net.Addr) (net.Conn, error) {
	dialer := r.Net.CreateDialer(&net.Dialer{
		LocalAddr: laddr,
		// Enable SO_REUSEADDR and SO_REUSEPORT where needed to let multiple connnections
		// bind to the same relay address.
		Control: reuseport.Control,
	})

	return dialer.Dial(network, raddr.String())
}
