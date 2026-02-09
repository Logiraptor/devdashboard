package ralph

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"devdeploy/internal/beads"
)

// LoopConfig configures the ralph autonomous work loop.
type LoopConfig struct {
	WorkDir       string
	Project       string
	Labels        []string
	MaxIterations int
	DryRun        bool
	Verbose       bool

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
}

// Run executes the ralph autonomous work loop. It picks beads, dispatches
// agents, assesses outcomes, and tracks consecutive failures. The loop stops
// when there are no more beads, the max iteration count is reached, the
// context is cancelled, or 3 consecutive failures occur.
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	logger := log.New(out, "ralph: ", 0)

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
			return RunAgent(ctx, cfg.WorkDir, prompt)
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

	for i := 0; i < cfg.MaxIterations; i++ {
		// Check for context cancellation before each iteration.
		if ctx.Err() != nil {
			logger.Printf("context cancelled, stopping")
			break
		}

		// 1. Pick next bead.
		bead, err := pickNext()
		if err != nil {
			return summary, fmt.Errorf("iteration %d: picking bead: %w", i+1, err)
		}
		if bead == nil {
			logger.Printf("no ready beads, done")
			break
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
		case OutcomeQuestion:
			summary.Questions++
			consecutiveFailures = 0
		case OutcomeFailure:
			summary.Failed++
			consecutiveFailures++
		case OutcomeTimeout:
			summary.TimedOut++
			consecutiveFailures++
		}

		if consecutiveFailures >= 3 {
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

	// Final summary.
	logger.Printf("done: %d iterations, %d succeeded, %d questions, %d failed, %d timed out",
		summary.Iterations, summary.Succeeded, summary.Questions, summary.Failed, summary.TimedOut)

	return summary, nil
}
