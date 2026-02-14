// Package ralph implements the autonomous agent work loop.
// core.go provides a simplified parallel execution loop.
package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// Core orchestrates parallel agent execution for a bead tree.
type Core struct {
	// WorkDir is the root repository directory.
	WorkDir string

	// RootBead is the epic or single bead to complete.
	// If set, only ready children of this bead are processed.
	// If empty, all ready beads are processed.
	RootBead string

	// MaxParallel is the maximum concurrent agents.
	// 0 or 1 means sequential execution.
	MaxParallel int

	// AgentTimeout is the per-agent execution timeout.
	// Zero means use DefaultTimeout (10m).
	AgentTimeout time.Duration

	// Output is where logs are written. Defaults to os.Stdout.
	Output io.Writer

	// Test hooks (nil means use real implementations)
	RunBD       BDRunner
	FetchPrompt func(runBD BDRunner, workDir, beadID string) (*PromptData, error)
	Render      func(data *PromptData) (string, error)
	Execute     func(ctx context.Context, workDir, prompt string) (*AgentResult, error)
	AssessFn    func(workDir, beadID string, result *AgentResult) (Outcome, string)
}

// CoreResult holds the aggregate results of a Core.Run invocation.
type CoreResult struct {
	Succeeded int
	Questions int
	Failed    int
	TimedOut  int
	Duration  time.Duration
}

// Run executes the ralph loop until no more beads are ready.
// Each iteration:
//  1. Queries ready beads (filtered by RootBead if set)
//  2. Executes beads in parallel (up to MaxParallel)
//  3. Merges results back to the main branch
func (c *Core) Run(ctx context.Context) (*CoreResult, error) {
	start := time.Now()
	result := &CoreResult{}

	out := c.Output
	if out == nil {
		out = os.Stdout
	}

	// Initialize worktree manager for parallel execution
	var wtMgr *WorktreeManager
	if c.MaxParallel > 1 {
		var err error
		wtMgr, err = NewWorktreeManager(c.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("creating worktree manager: %w", err)
		}
	}

	for {
		// Check context cancellation
		if ctx.Err() != nil {
			break
		}

		// 1. Query ready beads
		ready, err := c.readyBeads()
		if err != nil {
			return nil, fmt.Errorf("fetching ready beads: %w", err)
		}
		if len(ready) == 0 {
			break // No more work
		}

		writef(out, "Found %d ready bead(s)\n", len(ready))

		// 2. Execute in parallel (limited by MaxParallel)
		batchSize := len(ready)
		if c.MaxParallel > 0 && batchSize > c.MaxParallel {
			batchSize = c.MaxParallel
		}
		batch := ready[:batchSize]

		results := c.executeParallel(ctx, wtMgr, batch, out)

		// 3. Process results and merge back
		for _, r := range results {
			switch r.Outcome {
			case OutcomeSuccess:
				result.Succeeded++
			case OutcomeQuestion:
				result.Questions++
			case OutcomeFailure:
				result.Failed++
			case OutcomeTimeout:
				result.TimedOut++
			}

			// Merge successful work back if using worktrees
			if r.WorktreePath != "" && r.Outcome == OutcomeSuccess && wtMgr != nil {
				if err := c.mergeBack(ctx, wtMgr, r); err != nil {
					writef(out, "  warning: merge failed for %s: %v\n", r.BeadID, err)
				}
			}

			// Clean up worktree
			if r.WorktreePath != "" && wtMgr != nil {
				if err := wtMgr.RemoveWorktree(r.WorktreePath); err != nil {
					writef(out, "  warning: failed to remove worktree: %v\n", err)
				}
			}
		}
	}

	result.Duration = time.Since(start)

	// Print summary
	writef(out, "\nCore loop complete:\n")
	if result.Succeeded > 0 {
		writef(out, "  ✓ %d beads completed\n", result.Succeeded)
	}
	if result.Questions > 0 {
		writef(out, "  ? %d questions created\n", result.Questions)
	}
	if result.Failed > 0 {
		writef(out, "  ✗ %d failures\n", result.Failed)
	}
	if result.TimedOut > 0 {
		writef(out, "  ⏱ %d timeouts\n", result.TimedOut)
	}
	writef(out, "  Duration: %s\n", formatDuration(result.Duration))

	return result, nil
}

// beadExecResult holds the outcome of executing a single bead.
type beadExecResult struct {
	BeadID       string
	Outcome      Outcome
	Duration     time.Duration
	WorktreePath string
	BranchName   string
}

// readyBeads fetches beads that are ready to work on.
func (c *Core) readyBeads() ([]beads.Bead, error) {
	runner := c.RunBD
	if runner == nil {
		runner = bd.Run
	}

	args := []string{"ready", "--json"}
	if c.RootBead != "" {
		args = append(args, "--parent", c.RootBead)
	}

	out, err := runner(c.WorkDir, args...)
	if err != nil {
		return nil, err
	}

	return parseReadyBeads(out)
}

// executeParallel runs agents for a batch of beads concurrently.
func (c *Core) executeParallel(ctx context.Context, wtMgr *WorktreeManager, batch []beads.Bead, out io.Writer) []beadExecResult {
	results := make([]beadExecResult, len(batch))
	var wg sync.WaitGroup

	for i, bead := range batch {
		wg.Add(1)
		go func(idx int, b beads.Bead) {
			defer wg.Done()
			results[idx] = c.executeBead(ctx, wtMgr, &b, out)
		}(i, bead)
	}

	wg.Wait()
	return results
}

// executeBead runs an agent for a single bead.
func (c *Core) executeBead(ctx context.Context, wtMgr *WorktreeManager, bead *beads.Bead, out io.Writer) beadExecResult {
	start := time.Now()
	result := beadExecResult{BeadID: bead.ID}

	// Determine execution directory
	execDir := c.WorkDir
	if wtMgr != nil {
		worktreePath, branchName, err := wtMgr.CreateWorktree(bead.ID)
		if err != nil {
			writef(out, "[%s] failed to create worktree: %v\n", bead.ID, err)
			result.Outcome = OutcomeFailure
			result.Duration = time.Since(start)
			return result
		}
		execDir = worktreePath
		result.WorktreePath = worktreePath
		result.BranchName = branchName
	}

	// Fetch prompt data
	fetchPrompt := c.FetchPrompt
	if fetchPrompt == nil {
		fetchPrompt = FetchPromptData
	}
	promptData, err := fetchPrompt(c.RunBD, c.WorkDir, bead.ID)
	if err != nil {
		writef(out, "[%s] failed to fetch prompt: %v\n", bead.ID, err)
		result.Outcome = OutcomeFailure
		result.Duration = time.Since(start)
		return result
	}

	// Render prompt
	render := c.Render
	if render == nil {
		render = RenderPrompt
	}
	prompt, err := render(promptData)
	if err != nil {
		writef(out, "[%s] failed to render prompt: %v\n", bead.ID, err)
		result.Outcome = OutcomeFailure
		result.Duration = time.Since(start)
		return result
	}

	// Execute agent
	var agentResult *AgentResult
	if c.Execute != nil {
		agentResult, err = c.Execute(ctx, execDir, prompt)
	} else {
		var opts []Option
		if c.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(c.AgentTimeout))
		}
		agentResult, err = RunAgent(ctx, execDir, prompt, opts...)
	}
	if err != nil {
		writef(out, "[%s] agent execution error: %v\n", bead.ID, err)
		result.Outcome = OutcomeFailure
		result.Duration = time.Since(start)
		return result
	}

	// Assess outcome
	assessFn := c.AssessFn
	if assessFn == nil {
		assessFn = func(wd, id string, r *AgentResult) (Outcome, string) {
			return Assess(wd, id, r, nil)
		}
	}
	outcome, summary := assessFn(c.WorkDir, bead.ID, agentResult)

	result.Outcome = outcome
	result.Duration = agentResult.Duration

	writef(out, "[%s] %s → %s (%s)\n", bead.ID, bead.Title, outcome, formatDuration(result.Duration))
	if summary != "" && outcome != OutcomeSuccess {
		writef(out, "  %s\n", summary)
	}

	return result
}

// mergeBack merges a worktree branch back into the main branch.
func (c *Core) mergeBack(ctx context.Context, wtMgr *WorktreeManager, r beadExecResult) error {
	return MergeWithAgentResolution(
		ctx,
		wtMgr.SrcRepo(),
		wtMgr.Branch(),
		r.BranchName,
		r.BeadID,
		"", // beadTitle not needed
		c.AgentTimeout,
	)
}

// Run is a compatibility wrapper that runs Core using the legacy LoopConfig.
// This allows TUI and other code depending on the old interface to continue working.
// TODO(lbn.6): Remove this when TUI is extracted to its own subpackage.
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Determine the root bead from config
	rootBead := cfg.Epic
	if cfg.TargetBead != "" {
		rootBead = cfg.TargetBead
	}

	// Create Core from LoopConfig
	core := &Core{
		WorkDir:      cfg.WorkDir,
		RootBead:     rootBead,
		MaxParallel:  cfg.Concurrency,
		AgentTimeout: cfg.AgentTimeout,
		Output:       cfg.Output,
	}

	// Wire up test hooks if provided
	if cfg.FetchPrompt != nil {
		core.FetchPrompt = func(_ BDRunner, workDir, beadID string) (*PromptData, error) {
			return cfg.FetchPrompt(beadID)
		}
	}
	if cfg.Render != nil {
		core.Render = cfg.Render
	}
	if cfg.Execute != nil {
		core.Execute = func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return cfg.Execute(ctx, prompt)
		}
	}
	if cfg.AssessFn != nil {
		core.AssessFn = func(workDir, beadID string, result *AgentResult) (Outcome, string) {
			return cfg.AssessFn(beadID, result)
		}
	}

	// Run Core
	result, err := core.Run(ctx)
	if err != nil {
		return nil, err
	}

	// Convert CoreResult to RunSummary
	summary := &RunSummary{
		Iterations: result.Succeeded + result.Questions + result.Failed + result.TimedOut,
		Succeeded:  result.Succeeded,
		Questions:  result.Questions,
		Failed:     result.Failed,
		TimedOut:   result.TimedOut,
		Duration:   result.Duration,
	}

	// Determine stop reason
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			summary.StopReason = StopWallClock
		} else {
			summary.StopReason = StopContextCancelled
		}
	} else {
		summary.StopReason = StopNormal
	}

	return summary, nil
}
