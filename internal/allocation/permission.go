package allocation

import (
	"fmt"
	"time"

	"github.com/pion/stun"
)

const permissionTimeout = time.Duration(5) * time.Minute

// Permission represents a TURN permission. TURN permissions mimic the address-restricted
// filtering mechanism of NATs that comply with [RFC4787].
// https://tools.ietf.org/html/rfc5766#section-2.3
type Permission struct {
	Addr          *stun.TransportAddr
	allocation    *Allocation
	lifetimeTimer *time.Timer
}

func (p *Permission) start() {
	p.lifetimeTimer = time.AfterFunc(permissionTimeout, func() {
		if !p.allocation.RemovePermission(p.Addr) {
			fmt.Printf("Failed to remove permission for %v %v \n", p.Addr, p.allocation.fiveTuple)
		}
	})
}

func (p *Permission) refresh() {
	if !p.lifetimeTimer.Reset(permissionTimeout) {
		fmt.Printf("Failed to reset permission timer for %v %v \n", p.Addr, p.allocation.fiveTuple)
	}
}
