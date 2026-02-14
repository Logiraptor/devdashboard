package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"devdeploy/internal/beads"
)

// Batcher yields batches of beads for parallel execution.
// Each call to Next() returns the next batch, or an error.
// When no more batches are available, Next() returns (nil, nil).
type Batcher interface {
	Next() ([]beads.Bead, error)
}

// Runner executes beads in parallel batches using a batcher.
type Runner struct {
	batcher     Batcher
	cfg         LoopConfig
	summary     *RunSummary
	mu          sync.Mutex
	fetchPrompt func(beadID string) (*PromptData, error)
	render      func(data *PromptData) (string, error)
	execute     func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn    func(beadID string, result *AgentResult) (Outcome, string)
	out         io.Writer
}

// NewRunner creates a new Runner with the given batcher and configuration.
func NewRunner(batcher Batcher, cfg LoopConfig) *Runner {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	// Set up prompt fetching
	fetchPrompt := cfg.FetchPrompt
	if fetchPrompt == nil {
		fetchPrompt = func(beadID string) (*PromptData, error) {
			return FetchPromptData(nil, cfg.WorkDir, beadID)
		}
	}

	// Set up rendering
	render := cfg.Render
	if render == nil {
		render = RenderPrompt
	}

	// Set up execution
	execute := cfg.Execute
	if execute == nil {
		execute = func(ctx context.Context, prompt string) (*AgentResult, error) {
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
		}
	}

	// Set up assessment
	assessFn := cfg.AssessFn
	if assessFn == nil {
		assessFn = func(beadID string, result *AgentResult) (Outcome, string) {
			return Assess(cfg.WorkDir, beadID, result, nil)
		}
	}

	return &Runner{
		batcher:     batcher,
		cfg:         cfg,
		summary:     &RunSummary{},
		fetchPrompt: fetchPrompt,
		render:      render,
		execute:     execute,
		assessFn:    assessFn,
		out:         out,
	}
}

// Run executes beads from the batcher in parallel batches.
// For each batch, it fans out to goroutines (one per bead).
// Each goroutine: fetch prompt, execute agent, assess outcome.
// Collects results, updates summary, and checks stop conditions.
func (r *Runner) Run(ctx context.Context) (*RunSummary, error) {
	start := time.Now()
	batchNum := 0

	// Apply wall-clock timeout if configured
	wallTimeout := r.cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallTimeout)
	defer cancel()

	for {
		// Check stop conditions
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				r.summary.StopReason = StopWallClock
			} else {
				r.summary.StopReason = StopContextCancelled
			}
			break
		}

		// Check max batches (if MaxIterations is set, treat as max batches)
		if r.cfg.MaxIterations > 0 && batchNum >= r.cfg.MaxIterations {
			r.summary.StopReason = StopMaxIterations
			break
		}

		// Get next batch from batcher
		batch, err := r.batcher.Next()
		if err != nil {
			return nil, fmt.Errorf("batcher error: %w", err)
		}
		if batch == nil {
			// No more batches
			if batchNum == 0 {
				writef(r.out, "No batches available\n")
			} else {
				writef(r.out, "\nNo more batches after batch %d\n", batchNum)
			}
			r.summary.StopReason = StopNormal
			break
		}

		if len(batch) == 0 {
			// Empty batch, skip it
			batchNum++
			continue
		}

		batchNum++
		writef(r.out, "Batch %d: dispatching %d bead(s) in parallel\n", batchNum, len(batch))

		// Fan out to goroutines (one per bead)
		var wg sync.WaitGroup
		for i := range batch {
			wg.Add(1)
			go func(bead *beads.Bead) {
				defer wg.Done()
				r.executeBead(ctx, bead)
			}(&batch[i])
		}

		// Wait for all beads in this batch to complete
		wg.Wait()
	}

	// Calculate total duration
	r.summary.Duration = time.Since(start)

	// Count remaining beads
	remainingBeads := countRemainingBeads(r.cfg)

	// Print final summary
	writef(r.out, "\n%s\n", formatSummary(r.summary, remainingBeads))

	return r.summary, nil
}

// executeBead executes a single bead: fetch prompt, execute agent, assess outcome.
func (r *Runner) executeBead(ctx context.Context, bead *beads.Bead) {
	// Fetch prompt data
	promptData, err := r.fetchPrompt(bead.ID)
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to fetch prompt for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		return
	}

	// Render prompt
	prompt, err := r.render(promptData)
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to render prompt for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		return
	}

	// Execute agent
	result, err := r.execute(ctx, prompt)
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to run agent for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		return
	}

	// Assess outcome
	outcome, outcomeSummary := r.assessFn(bead.ID, result)

	// Update summary
	r.mu.Lock()
	r.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		r.summary.Succeeded++
	case OutcomeQuestion:
		r.summary.Questions++
	case OutcomeFailure:
		r.summary.Failed++
	case OutcomeTimeout:
		r.summary.TimedOut++
	}

	// Log iteration
	writef(r.out, "%s\n", formatIterationLog(
		r.summary.Iterations,
		r.cfg.MaxIterations,
		bead.ID,
		bead.Title,
		outcome,
		result.Duration,
		outcomeSummary,
	))

	// Verbose mode output
	if r.cfg.Verbose {
		printVerboseOutput(r.out, result)
	}

	r.mu.Unlock()
}
