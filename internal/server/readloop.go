package server

import (
	"net"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v2/internal/allocation"
)

type State struct {
	// Connection State
	Conn net.PacketConn

	// Server State
	AllocationManager *allocation.Manager
	Connect           func(conn net.PacketConn, remoteAddr net.Addr) (net.PacketConn, error)
	Nonces            *sync.Map
	InboundMTU        int
	Log               logging.LeveledLogger

	// User Configuration
	AuthHandler        func(username string, realm string, srcAddr net.Addr) (key []byte, ok bool)
	Realm              string
	ChannelBindTimeout time.Duration
}

func ReadLoop(r State) {
	buf := make([]byte, r.InboundMTU)
	for {
		n, addr, err := r.Conn.ReadFrom(buf)
		switch {
		case err != nil:
			r.Log.Debugf("exit read loop on error: %s", err.Error())
			return
		case n >= r.InboundMTU:
			r.Log.Debugf("read bytes exceeded MTU, packet is possibly truncated")
		}

		if err := HandleRequest(Request{
			State:   r,
			SrcAddr: addr,
			Buff:    buf[:n],
		}); err != nil {
			r.Log.Errorf("error when handling datagram: %v", err)
		}
	}
}
