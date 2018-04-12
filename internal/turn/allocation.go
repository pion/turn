package turnServer

import (
	"sync"
	"time"

	"gitlab.com/pions/pion/turn/internal/relay"
)

const (
	maxPermissions  = 10
	maxChannelBinds = 10
)

type Allocation struct {
	fiveTuple       *relayServer.FiveTuple
	listeningPort   int
	username        string
	lifetime        uint32
	permissionsLock sync.RWMutex
	permissions     map[string]Permission
	channelBindings map[int]relayServer.ChannelBind
}

func (a *Allocation) AddPermission(p *Permission, lifetime time.Duration) {
	// a.permissions[string(p.IP)].timer = time.AfterFunc(lifetime, func() {
	// 	delete
	// })
}
