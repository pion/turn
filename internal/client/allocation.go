// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun"
)

// AllocationConfig is a set of configuration params use by NewUDPConn and NewTCPAllocation
type AllocationConfig struct {
	Client      Client
	RelayedAddr net.Addr
	Integrity   stun.MessageIntegrity
	Nonce       stun.Nonce
	Lifetime    time.Duration
	Log         logging.LeveledLogger
}

type allocation struct {
	client            Client                // Read-only
	relayedAddr       net.Addr              // Read-only
	permMap           *permissionMap        // Thread-safe
	integrity         stun.MessageIntegrity // Read-only
	_nonce            stun.Nonce            // Needs mutex x
	_lifetime         time.Duration         // Needs mutex x
	refreshAllocTimer *PeriodicTimer        // Thread-safe
	refreshPermsTimer *PeriodicTimer        // Thread-safe
	readTimer         *time.Timer           // Thread-safe
	mutex             sync.RWMutex          // Thread-safe
	log               logging.LeveledLogger // Read-only
}
