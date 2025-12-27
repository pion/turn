// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"bytes"
	"encoding/gob"
	"net"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4/internal/proto"
)

// ChannelBind represents a TURN Channel
// See: https://tools.ietf.org/html/rfc5766#section-2.5
type ChannelBind struct {
	Peer   net.Addr
	Number proto.ChannelNumber

	allocation    *Allocation
	lifetimeTimer *time.Timer
	log           logging.LeveledLogger
	expiresAt     time.Time
}
type serializedChannelBind struct {
	Peer      string
	Number    proto.ChannelNumber
	Protocol  Protocol
	ExpiresAt time.Time
}

func (c *ChannelBind) serialize() *serializedChannelBind {
	return &serializedChannelBind{
		Peer:      c.Peer.String(),
		Number:    c.Number,
		Protocol:  c.allocation.Protocol,
		ExpiresAt: c.expiresAt,
	}
}

func (c *ChannelBind) deserialize(serialized *serializedChannelBind) error {
	network := strings.ToLower(serialized.Protocol.String())
	switch serialized.Protocol {
	case UDP:
		peerAddr, err := net.ResolveUDPAddr(network, serialized.Peer)
		if err != nil {
			return err
		}
		c.Peer = peerAddr
	case TCP:
		peerAddr, err := net.ResolveTCPAddr(network, serialized.Peer)
		if err != nil {
			return err
		}
		c.Peer = peerAddr
	default:
		return errUnsupportedProtocol
	}
	c.expiresAt = serialized.ExpiresAt
	c.Number = serialized.Number
	remaningTime := time.Until(serialized.ExpiresAt)
	if remaningTime > 0 {
		c.start(remaningTime)
	}

	return nil
}

func (c *ChannelBind) MarshalBinary() ([]byte, error) {
	serialized := c.serialize()
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(serialized); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *ChannelBind) UnmarshalBinary(data []byte) error {
	var serialized serializedChannelBind
	dec := gob.NewDecoder(bytes.NewBuffer(data))
	if err := dec.Decode(&serialized); err != nil {
		return err
	}

	return c.deserialize(&serialized)
}

// NewChannelBind creates a new ChannelBind.
func NewChannelBind(number proto.ChannelNumber, peer net.Addr, log logging.LeveledLogger) *ChannelBind {
	return &ChannelBind{
		Number: number,
		Peer:   peer,
		log:    log,
	}
}

func (c *ChannelBind) start(lifetime time.Duration) {
	c.expiresAt = time.Now().Add(lifetime)
	c.lifetimeTimer = time.AfterFunc(lifetime, func() {
		if !c.allocation.RemoveChannelBind(c.Number) {
			c.log.Errorf("Failed to remove ChannelBind for %v %x %v", c.Number, c.Peer, c.allocation.fiveTuple)
		}
	})
}

func (c *ChannelBind) refresh(lifetime time.Duration) {
	c.expiresAt = time.Now().Add(lifetime)
	if !c.lifetimeTimer.Reset(lifetime) {
		c.log.Errorf("Failed to reset ChannelBind timer for %v %x %v", c.Number, c.Peer, c.allocation.fiveTuple)
	}
}

func (c *ChannelBind) stop() {
	if c.lifetimeTimer != nil {
		c.expiresAt = time.Now()
		c.lifetimeTimer.Stop()
	}
}
