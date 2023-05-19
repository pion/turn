// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"syscall"

	"github.com/pion/randutil"
	"github.com/pion/transport/v2"
	"github.com/pion/transport/v2/stdnet"
	"golang.org/x/sys/unix"
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

// Validate is called on server startup and confirms the RelayAddressGenerator is properly configured
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

// AllocatePacketConn generates a new PacketConn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorPortRange) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	if requestedPort != 0 {
		conn, err := r.Net.ListenPacket(network, fmt.Sprintf("%s:%d", r.Address, requestedPort))
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
		port := r.MinPort + uint16(r.Rand.Intn(int((r.MaxPort+1)-r.MinPort)))
		conn, err := r.Net.ListenPacket(network, fmt.Sprintf("%s:%d", r.Address, port))
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

// AllocatePacketConn generates a new PacketConn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorPortRange) AllocateListener(network string, requestedPort int) (net.Listener, net.Addr, error) {
	config := &net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var err error
		c.Control(func(fd uintptr) {
			err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEADDR|unix.SO_REUSEPORT, 1)
		})
		return err
	}}

	if requestedPort != 0 {
		listener, err := config.Listen(context.Background(), network, r.Address+":"+strconv.Itoa(requestedPort))
		if err != nil {
			return nil, nil, err
		}
		relayAddr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			return nil, nil, errNilConn
		}

		relayAddr.IP = r.RelayAddress
		return listener, relayAddr, nil
	}

	for try := 0; try < r.MaxRetries; try++ {
		port := r.MinPort + uint16(r.Rand.Intn(int((r.MaxPort+1)-r.MinPort)))
		listener, err := config.Listen(context.Background(), network, fmt.Sprintf("%s:%d", r.Address, port))
		if err != nil {
			continue
		}

		relayAddr, ok := listener.Addr().(*net.TCPAddr)
		if !ok {
			return nil, nil, errNilConn
		}

		relayAddr.IP = r.RelayAddress
		return listener, relayAddr, nil
	}

	return nil, nil, errMaxRetriesExceeded
}

// AllocateConn generates a new Conn to receive traffic on and the IP/Port to populate the allocation response with
func (r *RelayAddressGeneratorPortRange) AllocateConn(string, int) (net.Conn, net.Addr, error) {
	return nil, nil, errTODO
}
