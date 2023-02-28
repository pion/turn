/*
Package profiling provides easy access to the Go standard tooling. It is designed to the tracing is only included when -tags debug is present at compile / run time.

The production_stub.go and profiling.go mimic each other.
This types.go provides the common struct required in both cases.
*/
package profiling

import (
	"context"
	"os"
	"runtime/trace"

	"github.com/pion/logging"
)

// Profiling is the standard struct required for both production and debug compilation
//
//nolint:golint,unused
type Profiling struct {
	tracefile *os.File
	task      *trace.Task
	basectx   context.Context
	mLogger   logging.LeveledLogger
}
