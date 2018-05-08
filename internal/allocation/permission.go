package allocation

import (
	"fmt"
	"time"

	"github.com/pions/pkg/stun"
)

const permissionTimeout = time.Duration(5) * time.Minute

// Public
type Permission struct {
	Addr          *stun.TransportAddr
	allocation    *Allocation
	lifetimeTimer *time.Timer
}

func (p *Permission) start() {
	p.lifetimeTimer = time.AfterFunc(permissionTimeout, func() {
		if !p.allocation.RemovePermission(p.Addr) {
			fmt.Printf("Failed to remove permission for %v %v", p.Addr, p.allocation.fiveTuple)
		}
	})
}

func (p *Permission) refresh() {
	if !p.lifetimeTimer.Reset(permissionTimeout) {
		fmt.Printf("Failed to reset permission timer for %v %v", p.Addr, p.allocation.fiveTuple)
	}
}
