package ralph

import (
	"context"
	"fmt"
	"io"
	"iter"
	"time"

	"devdeploy/internal/beads"
)

// BeadBatcher yields batches of beads to process in parallel.
// Each call to the iterator returns:
// - A slice of beads to process (run in parallel)
// - false when no more beads are available
type BeadBatcher = iter.Seq[[]beads.Bead]

// RunConfig holds configuration for a ralph run.
// Simplified from current LoopConfig - only essential fields.
type RunConfig struct {
	WorkDir      string
	MaxBatches   int           // Max batches to process (0 = unlimited)
	AgentTimeout time.Duration // Per-agent timeout
	DryRun       bool
	Verbose      bool
	Output       io.Writer
}

// Run executes the ralph loop.
func Run(ctx context.Context, cfg RunConfig, batcher BeadBatcher) (*RunSummary, error) {
	return nil, fmt.Errorf("not implemented yet")
}
