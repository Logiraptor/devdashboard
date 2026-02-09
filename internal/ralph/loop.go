package ralph

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"time"

	"devdeploy/internal/beads"
)

// StopReason indicates why the ralph loop terminated.
type StopReason int

const (
	StopNormal           StopReason = iota // No more beads or all iterations completed.
	StopMaxIterations                      // Hit --max-iterations cap.
	StopConsecutiveFails                   // Too many consecutive failures.
	StopWallClock                          // Total --timeout wall-clock exceeded.
	StopContextCancelled                   // Context cancelled (e.g. SIGINT).
	StopAllBeadsSkipped                    // All available beads were skipped (retry detection).
)

// String returns a human-readable label for the stop reason.
func (r StopReason) String() string {
	switch r {
	case StopNormal:
		return "normal"
	case StopMaxIterations:
		return "max-iterations"
	case StopConsecutiveFails:
		return "consecutive-failures"
	case StopWallClock:
		return "wall-clock-timeout"
	case StopContextCancelled:
		return "context-cancelled"
	case StopAllBeadsSkipped:
		return "all-beads-skipped"
	default:
		return "unknown"
	}
}

// ExitCode returns a distinct process exit code for each stop reason.
func (r StopReason) ExitCode() int {
	switch r {
	case StopNormal:
		return 0
	case StopMaxIterations:
		return 2
	case StopConsecutiveFails:
		return 3
	case StopWallClock:
		return 4
	case StopContextCancelled:
		return 5
	case StopAllBeadsSkipped:
		return 6
	default:
		return 1
	}
}

// DefaultConsecutiveFailureLimit is the default number of consecutive failures
// before the loop stops.
const DefaultConsecutiveFailureLimit = 3

// DefaultWallClockTimeout is the default total wall-clock timeout for a ralph session.
const DefaultWallClockTimeout = 2 * time.Hour

// LoopConfig configures the ralph autonomous work loop.
type LoopConfig struct {
	WorkDir       string
	Project       string
	Labels        []string
	MaxIterations int
	DryRun        bool
	Verbose       bool

	// AgentTimeout is the per-agent execution timeout. Zero means use the
	// executor's DefaultTimeout (10m).
	AgentTimeout time.Duration

	// ConsecutiveFailureLimit stops the loop after N consecutive failures.
	// Zero means use DefaultConsecutiveFailureLimit (3).
	ConsecutiveFailureLimit int

	// Timeout is the total wall-clock timeout for the entire ralph session.
	// Zero means use DefaultWallClockTimeout (2h).
	Timeout time.Duration

	// Test hooks — nil means use real implementations.
	PickNext    func() (*beads.Bead, error)
	FetchPrompt func(beadID string) (*PromptData, error)
	Render      func(data *PromptData) (string, error)
	Execute     func(ctx context.Context, prompt string) (*AgentResult, error)
	AssessFn    func(beadID string, result *AgentResult) (Outcome, string)
	SyncFn      func() error
	Output      io.Writer // defaults to os.Stdout
}

// RunSummary holds aggregate results across all iterations.
type RunSummary struct {
	Iterations int
	Succeeded  int
	Questions  int
	Failed     int
	TimedOut   int
	Skipped    int
	StopReason StopReason
}

// Run executes the ralph autonomous work loop. It picks beads, dispatches
// agents, assesses outcomes, and enforces safety guards. The loop stops when:
//   - no more beads are available
//   - max iteration count is reached
//   - the context is cancelled
//   - N consecutive failures occur
//   - the total wall-clock timeout expires
//   - all available beads have been skipped (same-bead retry detection)
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	logger := log.New(out, "ralph: ", 0)

	// Resolve defaults.
	consecutiveLimit := cfg.ConsecutiveFailureLimit
	if consecutiveLimit <= 0 {
		consecutiveLimit = DefaultConsecutiveFailureLimit
	}

	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, wallTimeout)
	defer cancelWall()

	pickNext := cfg.PickNext
	if pickNext == nil {
		picker := &BeadPicker{
			WorkDir: cfg.WorkDir,
			Project: cfg.Project,
			Labels:  cfg.Labels,
		}
		pickNext = picker.Next
	}

	fetchPrompt := cfg.FetchPrompt
	if fetchPrompt == nil {
		fetchPrompt = func(beadID string) (*PromptData, error) {
			return FetchPromptData(nil, cfg.WorkDir, beadID)
		}
	}

	render := cfg.Render
	if render == nil {
		render = RenderPrompt
	}

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

	assessFn := cfg.AssessFn
	if assessFn == nil {
		assessFn = func(beadID string, result *AgentResult) (Outcome, string) {
			return Assess(cfg.WorkDir, beadID, result)
		}
	}

	syncFn := cfg.SyncFn
	if syncFn == nil {
		syncFn = func() error {
			cmd := exec.Command("bd", "sync")
			cmd.Dir = cfg.WorkDir
			return cmd.Run()
		}
	}

	summary := &RunSummary{}
	consecutiveFailures := 0
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)

	for i := 0; i < cfg.MaxIterations; i++ {
		// Guard: context cancellation / wall-clock timeout.
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				summary.StopReason = StopWallClock
				logger.Printf("wall-clock timeout (%s) exceeded, stopping", wallTimeout)
			} else {
				summary.StopReason = StopContextCancelled
				logger.Printf("context cancelled, stopping")
			}
			break
		}

		// 1. Pick next bead.
		bead, err := pickNext()
		if err != nil {
			return summary, fmt.Errorf("iteration %d: picking bead: %w", i+1, err)
		}
		if bead == nil {
			logger.Printf("no ready beads, done")
			summary.StopReason = StopNormal
			break
		}

		// Guard: same-bead retry detection.
		// If the same bead that just failed is picked again, skip it.
		if lastFailedBeadID != "" && bead.ID == lastFailedBeadID {
			skippedBeads[bead.ID] = true
			summary.Skipped++
			logger.Printf("skipping %s (same bead failed last iteration)", bead.ID)
			lastFailedBeadID = "" // reset so we don't skip indefinitely

			// Check if we should stop: if we've skipped beads and the picker
			// keeps returning skipped beads, we're in an infinite loop.
			// Try one more pick; if that's also skipped, stop.
			retryBead, retryErr := pickNext()
			if retryErr != nil {
				return summary, fmt.Errorf("iteration %d: picking bead (retry): %w", i+1, retryErr)
			}
			if retryBead == nil {
				logger.Printf("no ready beads after skip, done")
				summary.StopReason = StopNormal
				break
			}
			if skippedBeads[retryBead.ID] {
				summary.Skipped++
				summary.StopReason = StopAllBeadsSkipped
				logger.Printf("all available beads have been skipped, stopping")
				break
			}
			bead = retryBead
		}

		if cfg.Verbose {
			logger.Printf("iteration %d/%d: picked %s — %s (P%d)",
				i+1, cfg.MaxIterations, bead.ID, bead.Title, bead.Priority)
		}

		// Dry-run: print what would be done without executing.
		if cfg.DryRun {
			logger.Printf("[dry-run] would work on %s — %s (P%d)",
				bead.ID, bead.Title, bead.Priority)
			summary.Iterations++
			break
		}

		// 2. Fetch full bead data and render prompt.
		promptData, err := fetchPrompt(bead.ID)
		if err != nil {
			return summary, fmt.Errorf("iteration %d: fetching prompt data for %s: %w",
				i+1, bead.ID, err)
		}

		prompt, err := render(promptData)
		if err != nil {
			return summary, fmt.Errorf("iteration %d: rendering prompt for %s: %w",
				i+1, bead.ID, err)
		}

		// 3. Execute agent.
		result, err := execute(ctx, prompt)
		if err != nil {
			return summary, fmt.Errorf("iteration %d: running agent for %s: %w",
				i+1, bead.ID, err)
		}

		// 4. Assess outcome.
		outcome, outcomeSummary := assessFn(bead.ID, result)

		logger.Printf("iteration %d/%d: %s — %s [%s] %s",
			i+1, cfg.MaxIterations, bead.ID, bead.Title, outcome, outcomeSummary)

		// 5. Update counters.
		summary.Iterations++
		switch outcome {
		case OutcomeSuccess:
			summary.Succeeded++
			consecutiveFailures = 0
			lastFailedBeadID = ""
		case OutcomeQuestion:
			summary.Questions++
			consecutiveFailures = 0
			lastFailedBeadID = ""
		case OutcomeFailure:
			summary.Failed++
			consecutiveFailures++
			lastFailedBeadID = bead.ID
		case OutcomeTimeout:
			summary.TimedOut++
			consecutiveFailures++
			lastFailedBeadID = bead.ID
		}

		// Guard: consecutive failure limit.
		if consecutiveFailures >= consecutiveLimit {
			summary.StopReason = StopConsecutiveFails
			logger.Printf("too many consecutive failures (%d), stopping", consecutiveFailures)
			break
		}

		// 6. Sync beads state.
		if err := syncFn(); err != nil {
			if cfg.Verbose {
				logger.Printf("bd sync warning: %v", err)
			}
		}
	}

	// If we exhausted all iterations without an earlier stop reason, set it.
	if summary.StopReason == StopNormal && summary.Iterations >= cfg.MaxIterations {
		summary.StopReason = StopMaxIterations
		logger.Printf("reached max iterations (%d), stopping", cfg.MaxIterations)
	}

	// Final summary.
	logger.Printf("done: %d iterations, %d succeeded, %d questions, %d failed, %d timed out, %d skipped [stop: %s]",
		summary.Iterations, summary.Succeeded, summary.Questions, summary.Failed, summary.TimedOut, summary.Skipped, summary.StopReason)

	return summary, nil
}
