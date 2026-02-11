package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Run executes the ralph autonomous work loop. It picks beads, dispatches
// agents, assesses outcomes, and enforces safety guards. The loop stops when:
//   - no more beads are available
//   - max iteration count is reached
//   - the context is cancelled
//   - N consecutive failures occur
//   - the total wall-clock timeout expires
//   - all available beads have been skipped (same-bead retry detection)
//
// When Concurrency > 1, agents run in parallel, each in its own git worktree.
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Use WaveOrchestrator by default
	orchestrator, err := NewWaveOrchestrator(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating wave orchestrator: %w", err)
	}
	return orchestrator.Run(ctx)
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
