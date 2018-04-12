package stunServer

import (
	"net"
	"time"
)

// Public
type Permission struct {
	IP    net.IP
	Port  int
	timer *time.Timer
}

func NewPermission(ip net.IP, port int) *Permission {
	p := &Permission{}
	p.IP = ip
	p.Port = port
}

func (p *Permission) Refresh(lifetime time.Duration) {
	if p.timer == nil {
		return
	}

	if !p.timer.Stop() {
		<-p.timer.C
	}

	p.timer.Reset(lifetime)
}
