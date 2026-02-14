package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Run executes the ralph loop with the given configuration.
// This is the main entry point for running the ralph loop.
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	return RunLegacy(ctx, cfg)
}

// RunLegacy is the legacy Run function, temporarily renamed to make room for the new Run API in runner.go.
// TODO: Migrate callers to the new Run API and remove this function.
func RunLegacy(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// If PickNext is provided, use the sequential loop (typically for testing)
	// This allows tests to control bead selection without requiring a git repo
	if cfg.PickNext != nil {
		return runSequential(ctx, cfg)
	}

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
