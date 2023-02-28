//go:build !debug

package profiling

import (
	"fmt"
	"runtime/trace"

	"github.com/pion/logging"
)

// SayHello just provides easy indication of which version has been included
func SayHello() {
	fmt.Println("welcome to production")
}

// NewProfiling provides a stub function when in production, giving back an empty struct
func NewProfiling(filename string, mLogger logging.LeveledLogger) *Profiling {
	return &Profiling{}
}

// OpenTracing provides a stub function when in production
func (p *Profiling) OpenTracing(filename string, topLevel string) {}

// CloseTracing provides a stub function when in production
func (p *Profiling) CloseTracing() {}

// SetRegion provides a stub  function when in production
func (p *Profiling) SetRegion(regionname string) *trace.Region { return &trace.Region{} }
