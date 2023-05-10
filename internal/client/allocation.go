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

type Allocation struct {
	client            Client                // read-only
	relayedAddr       net.Addr              // read-only
	permMap           *permissionMap        // thread-safe
	bindingMgr        *bindingManager       // Thread-safe
	integrity         stun.MessageIntegrity // read-only
	_nonce            stun.Nonce            // needs mutex x
	_lifetime         time.Duration         // needs mutex x
	refreshAllocTimer *PeriodicTimer        // thread-safe
	refreshPermsTimer *PeriodicTimer        // thread-safe
	readTimer         *time.Timer           // thread-safe
	mutex             sync.RWMutex          // thread-safe
	log               logging.LeveledLogger // read-only
}
