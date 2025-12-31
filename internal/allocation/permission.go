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
)

const DefaultPermissionTimeout = time.Duration(5) * time.Minute

// Permission represents a TURN permission. TURN permissions mimic the address-restricted
// filtering mechanism of NATs that comply with [RFC4787].
// See: https://tools.ietf.org/html/rfc5766#section-2.3
type Permission struct {
	Addr          net.Addr
	allocation    *Allocation
	timeout       time.Duration
	lifetimeTimer *time.Timer
	log           logging.LeveledLogger
	expiresAt     time.Time
}
type serializedPermission struct {
	Addr      string
	ExpiresAt time.Time
	Protocol  Protocol
}

// NewPermission create a new Permission.
func NewPermission(addr net.Addr, log logging.LeveledLogger, timeout time.Duration) *Permission {
	return &Permission{
		Addr:    addr,
		log:     log,
		timeout: timeout,
	}
}

func (p *Permission) serialize() *serializedPermission {
	return &serializedPermission{
		Addr:      p.Addr.String(),
		ExpiresAt: p.expiresAt,
		Protocol:  p.allocation.Protocol,
	}
}

func (p *Permission) deserialize(serialized *serializedPermission) error {
	network := strings.ToLower(serialized.Protocol.String())
	switch serialized.Protocol {
	case UDP:
		permAddr, err := net.ResolveUDPAddr(network, serialized.Addr)
		if err != nil {
			return err
		}
		p.Addr = permAddr
	case TCP:
		permAddr, err := net.ResolveTCPAddr(network, serialized.Addr)
		if err != nil {
			return err
		}
		p.Addr = permAddr
	}
	remaningTime := time.Until(serialized.ExpiresAt)
	p.expiresAt = serialized.ExpiresAt
	if remaningTime > 0 {
		p.start(remaningTime)
	}

	return nil
}

func (p *Permission) MarshalBinary() ([]byte, error) {
	serialized := p.serialize()
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(*serialized); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (p *Permission) UnmarshalBinary(data []byte) error {
	var serialized serializedPermission
	enc := gob.NewDecoder(bytes.NewBuffer(data))
	if err := enc.Decode(&serialized); err != nil {
		return err
	}

	return p.deserialize(&serialized)
}

func (p *Permission) start(lifetime time.Duration) {
	p.expiresAt = time.Now().Add(lifetime)
	p.lifetimeTimer = time.AfterFunc(lifetime, func() {
		p.allocation.RemovePermission(p.Addr)
	})
}

func (p *Permission) refresh(lifetime time.Duration) {
	p.expiresAt = time.Now().Add(lifetime)
	if !p.lifetimeTimer.Reset(lifetime) {
		p.log.Errorf("Failed to reset permission timer for %v %v", p.Addr, p.allocation.fiveTuple)
	}
}

func (p *Permission) stop() {
	if p.lifetimeTimer != nil {
		p.expiresAt = time.Now()
		p.lifetimeTimer.Stop()
	}
}
