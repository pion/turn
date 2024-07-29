// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package turn

import (
	"net"

	"github.com/pion/logging"
	"github.com/pion/turn/v3/internal/offload"
)

// OffloadConfig defines various offload options
type OffloadConfig struct {
	// Logger is a leveled logger
	Log logging.LeveledLogger
	// Mechanisms are the offload mechanisms to be used. First element has the highest priority.
	// Available mechanisms are:
	// - "xdp": XDP/eBPF offload for UDP traffic
	// - "null": no offload
	Mechanisms []string
	// Interfaces on which to enable offload. Unless set, it is set to all available interfaces
	Interfaces []net.Interface
}

// InitOffload initializes offloading engine (e.g., eBPF kernel offload engine) to speed up networking
func InitOffload(o OffloadConfig) error {
	var err error
	offload.Engine, err = newEngine(o)
	if err != nil {
		return err
	}
	err = offload.Engine.Init()
	return err
}

// newEngine instantiates a new offload engine. It probes strategies until a fitting one is ousable one is found
func newEngine(opt OffloadConfig) (offload.OffloadEngine, error) {
	// set defaults
	if len(opt.Mechanisms) == 0 {
		opt.Mechanisms = []string{"xdp", "null"}
	}
	if len(opt.Interfaces) == 0 {
		ifs, err := net.Interfaces()
		if err != nil {
			return nil, err
		}
		opt.Interfaces = ifs
	}
	// iterate over mechanisms until a working solution is found
	var off offload.OffloadEngine
	var err error
	for _, m := range opt.Mechanisms {
		switch m {
		case "xdp":
			// try XDP/eBPF
			off, err = offload.NewXdpEngine(opt.Interfaces, opt.Log)
			// in case it is already running, restart it
			_, isExist := offload.Engine.(*offload.XdpEngine)
			if err != nil && isExist {
				offload.Engine.Shutdown()
				off, err = offload.NewXdpEngine(opt.Interfaces, opt.Log)
			}
		case "null":
			// no offload
			off, err = offload.NewNullEngine(opt.Log)
		default:
			opt.Log.Error("unsupported mechanism")
			off, err = nil, errUnsupportedOffloadMechanism
		}
		if off != nil && err == nil {
			break
		}
	}
	// fallback to no offload
	if err != nil {
		return offload.NewNullEngine(opt.Log)
	}
	return off, err
}

// ShutdownOffload shuts down the offloading engine
func ShutdownOffload() {
	offload.Engine.Shutdown()
}
