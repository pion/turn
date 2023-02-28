//go:build !debug

package profiling

import (
	"fmt"

	"runtime/trace"

	"github.com/pion/logging"
)

func SayHello() {
	fmt.Println("welcome to production")
}

func NewProfiling(filename string, mLogger logging.LeveledLogger) *Profiling {
	return &Profiling{}
}

func (p *Profiling) OpenTracing(filename string, topLevel string) {}

func (p *Profiling) CloseTracing() {}

func (p *Profiling) SetRegion(regionname string) *trace.Region {}
