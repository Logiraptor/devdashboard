package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"devdeploy/internal/beads"
)

// Run executes the ralph loop with the given configuration.
// This is the main entry point for running the ralph loop.
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Select batcher based on configuration
	var batcher BeadBatcher

	if cfg.PickNext != nil {
		// Test mode: use a custom batcher that wraps PickNext
		batcher = func(yield func([]beads.Bead) bool) {
			for {
				bead, err := cfg.PickNext()
				if err != nil || bead == nil {
					return
				}
				if !yield([]beads.Bead{*bead}) {
					return
				}
			}
		}
	} else if cfg.TargetBead != "" {
		// Targeted mode: work on a specific bead
		batcher = TargetedBatcher(cfg.WorkDir, cfg.TargetBead)
	} else if cfg.Epic != "" {
		// Epic mode: orchestrate children sequentially
		batcher = EpicBatcher(cfg.WorkDir, cfg.Epic)
	} else {
		// Default: wave mode (all ready beads in parallel)
		batcher = WaveBatcher(cfg.WorkDir, "")
	}

	// Create runner with selected batcher
	runner, err := NewRunner(batcher, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating runner: %w", err)
	}

	// Run the unified loop
	summary, err := runner.Run(ctx)
	if err != nil {
		return nil, err
	}

	// Epic mode: run opus verification after all children complete
	if cfg.Epic != "" && summary.Failed == 0 && summary.TimedOut == 0 && summary.Iterations > 0 {
		// Note: Epic verification logic would go here if needed
		// For now, we'll keep it simple and let the epic orchestrator handle it separately
		// if needed in the future
	}

	return summary, nil
}

// resolvedConfig holds resolved configuration values common to all loop types.
type resolvedConfig struct {
	out              io.Writer
	consecutiveLimit int
	wallTimeout      time.Duration
	fetchPrompt      func(beadID string) (*PromptData, error)
	render           func(data *PromptData) (string, error)
	assessFn         func(beadID string, result *AgentResult) (Outcome, string)
	syncFn           func() error
}

// resolveConfig resolves common configuration defaults and hooks.
func resolveConfig(cfg LoopConfig) resolvedConfig {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	consecutiveLimit := cfg.ConsecutiveFailureLimit
	if consecutiveLimit <= 0 {
		consecutiveLimit = DefaultConsecutiveFailureLimit
	}

	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
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

	assessFn := cfg.AssessFn
	if assessFn == nil {
		assessFn = func(beadID string, result *AgentResult) (Outcome, string) {
			return Assess(cfg.WorkDir, beadID, result, nil)
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

	return resolvedConfig{
		out:              out,
		consecutiveLimit: consecutiveLimit,
		wallTimeout:      wallTimeout,
		fetchPrompt:      fetchPrompt,
		render:           render,
		assessFn:         assessFn,
		syncFn:           syncFn,
	}
}

// createExecuteFn creates an execute function with formatter support.
// The formatter pointer can be nil if formatter tracking is not needed.
func createExecuteFn(cfg LoopConfig, out io.Writer, formatterPtr **LogFormatter) func(ctx context.Context, prompt string) (*AgentResult, error) {
	return func(ctx context.Context, prompt string) (*AgentResult, error) {
		var opts []Option
		if cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(cfg.AgentTimeout))
		}
		if !cfg.Verbose {
			if formatterPtr != nil {
				*formatterPtr = NewLogFormatter(out, false)
				opts = append(opts, WithStdoutWriter(*formatterPtr))
			}
		} else if out != os.Stdout {
			opts = append(opts, WithStdoutWriter(out))
		}
		return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
	}
}
