package allocation

import (
	"net"
	"time"

	"github.com/pion/logging"
)

const permissionTimeout = time.Duration(5) * time.Minute

// Permission represents a TURN permission. TURN permissions mimic the address-restricted
// filtering mechanism of NATs that comply with [RFC4787].
// https://tools.ietf.org/html/rfc5766#section-2.3
type Permission struct {
	Addr          net.Addr
	allocation    *Allocation
	lifetimeTimer *time.Timer
	log           logging.LeveledLogger
}

// NewPermission create a new Permission
func NewPermission(addr net.Addr, log logging.LeveledLogger) *Permission {
	return &Permission{
		Addr: addr,
		log:  log,
	}
}

func (p *Permission) start() {
	p.lifetimeTimer = time.AfterFunc(permissionTimeout, func() {
		if !p.allocation.RemovePermission(p.Addr) {
			p.log.Errorf("Failed to remove permission for %v %v", p.Addr, p.allocation.fiveTuple)
		}
	})
}

func (p *Permission) refresh() {
	if !p.lifetimeTimer.Reset(permissionTimeout) {
		p.log.Errorf("Failed to reset permission timer for %v %v", p.Addr, p.allocation.fiveTuple)
	}
}
