// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
)

// Protocol is an enum for relay protocol.
type Protocol uint8

// Network protocols for relay.
const (
	UDP Protocol = iota
	TCP
)

func (p Protocol) String() string {
	switch p {
	case UDP:
		return "UDP"
	case TCP:
		return "TCP"
	default:
		return ""
	}
}

// FiveTuple is the combination (client IP address and port, server IP
// address and port, and transport protocol (currently one of UDP,
// TCP, or TLS)) used to communicate between the client and the
// server.  The 5-tuple uniquely identifies this communication
// stream.  The 5-tuple also uniquely identifies the Allocation on
// the server.
type FiveTuple struct {
	Protocol
	SrcAddr, DstAddr net.Addr
}

type serializedFiveTuple struct {
	SrcIP    []byte
	SrcPort  uint16
	DstIP    []byte
	DstPort  uint16
	Protocol Protocol
}

// Equal asserts if two FiveTuples are equal.
func (f *FiveTuple) Equal(b *FiveTuple) bool {
	return f.Fingerprint() == b.Fingerprint()
}
func (f *FiveTuple) serialize() (*serializedFiveTuple, error) {
	srcIP, srcPort := netAddrIPAndPort(f.SrcAddr)
	dstIP, dstPort := netAddrIPAndPort(f.DstAddr)
	return &serializedFiveTuple{
		SrcIP:    srcIP,
		SrcPort:  srcPort,
		DstIP:    dstIP,
		DstPort:  dstPort,
		Protocol: f.Protocol,
	}, nil

}
func (f *FiveTuple) deserialize(s *serializedFiveTuple) error {
	switch s.Protocol {
	case UDP:
		f.SrcAddr = &net.UDPAddr{IP: s.SrcIP, Port: int(s.SrcPort)}
		f.DstAddr = &net.UDPAddr{IP: s.DstIP, Port: int(s.DstPort)}
	case TCP:
		f.SrcAddr = &net.TCPAddr{IP: s.SrcIP, Port: int(s.SrcPort)}
		f.DstAddr = &net.TCPAddr{IP: s.DstIP, Port: int(s.DstPort)}
	default:
		return fmt.Errorf("Unsupported protocol %v", s.Protocol)

	}
	f.Protocol = s.Protocol
	return nil
}
func (f *FiveTuple) MarshalBinary() ([]byte, error) {
	serialized, err := f.serialize()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(*serialized); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
func (f *FiveTuple) UnmarshalBinary(data []byte) error {
	var serialized serializedFiveTuple
	enc := gob.NewDecoder(bytes.NewBuffer(data))
	if err := enc.Decode(&serialized); err != nil {
		return err
	}
	return f.deserialize(&serialized)
}

// FiveTupleFingerprint is a comparable representation of a FiveTuple.
type FiveTupleFingerprint struct {
	SrcIP, DstIP     [16]byte
	SrcPort, DstPort uint16
	protocol         Protocol
}

// Fingerprint is the identity of a FiveTuple.
func (f *FiveTuple) Fingerprint() (fp FiveTupleFingerprint) {
	srcIP, srcPort := netAddrIPAndPort(f.SrcAddr)
	copy(fp.SrcIP[:], srcIP)
	fp.SrcPort = srcPort
	dstIP, dstPort := netAddrIPAndPort(f.DstAddr)
	copy(fp.DstIP[:], dstIP)
	fp.DstPort = dstPort
	fp.protocol = f.Protocol

	return
}

func netAddrIPAndPort(addr net.Addr) (net.IP, uint16) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		return a.IP.To16(), uint16(a.Port) // nolint:gosec // G115
	case *net.TCPAddr:
		return a.IP.To16(), uint16(a.Port) // nolint:gosec // G115
	default:
		return nil, 0
	}
}
