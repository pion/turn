//go:build debug

package profiling

import (
	"context"
	"fmt"
	"github.com/pion/logging"
	"os"
	"runtime/trace"
)

// SayHello just provides easy indication of which version has been included
func SayHello() {
	fmt.Println("welcome to debug")
}

// Open the tracing file - using a supplied or default file
// the top level tracing context is also supplied.
func (p *Profiling) OpenTracing(filename string, topLevel string) {
	var err error
	if len(filename) == 0 {
		filename = "trace.out"
	}
	p.tracefile, err = os.Create(filename)
	if err != nil {
		panic("unable to open trace file")
	}
	trace.Start(p.tracefile)
	p.basectx = context.TODO()
	_, p.task = trace.NewTask(p.basectx, topLevel)
	//p.mLogger.Debugf("Opened task, with output to file")
	fmt.Printf("Opened task, with output to file %s\n", filename)
}

func (p *Profiling) SetRegion(regionname string) *trace.Region {
	return trace.StartRegion(p.basectx, regionname)
}

// End the task, Stop tracing and close the tracing file.
// This is essential to make sure pprof can deal with it..
func (p *Profiling) CloseTracing() {
	p.task.End()
	trace.Stop()
	p.tracefile.Close()
	//p.mLogger.Debugf("Closed task, stopped trace and closed file")
	fmt.Println("Closed task, stopped trace and closed file")
}

func NewProfiling(filename string, mLogger logging.LeveledLogger) *Profiling {
	mProfiling := Profiling{}
	mProfiling.mLogger = mLogger
	mProfiling.OpenTracing(filename, "Request")
	return &mProfiling
}
