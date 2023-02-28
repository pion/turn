package profiling

import (
	"context"
	"os"
	"runtime/trace"

	"github.com/pion/logging"
)

type Profiling struct {
	tracefile *os.File
	task      *trace.Task
	basectx   context.Context
	mLogger   logging.LeveledLogger
}
