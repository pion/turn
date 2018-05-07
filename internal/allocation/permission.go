package allocation

import (
	"fmt"
	"time"

	"github.com/pions/pkg/stun"
)

// Public
type Permission struct {
	Addr       *stun.TransportAddr
	allocation *Allocation
	timer      *time.Timer
}

func (p *Permission) refresh() {
	fmt.Println("Refresh Permission!")
	// if p.timer == nil {
	// 	return
	// }

	// if !p.timer.Stop() {
	// 	<-p.timer.C
	// }

	// p.timer.Reset(lifetime)
}
