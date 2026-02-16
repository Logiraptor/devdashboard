// Package ralph implements the autonomous agent work loop.
// core.go provides a simplified parallel execution loop.
package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// ToolEvent represents a tool call event.
type ToolEvent struct {
	ID         string            // Unique identifier for this tool call
	Name       string            // Tool name
	Started    bool              // True for start events, false for end events
	Timestamp  time.Time         // When the event occurred
	Attributes map[string]string // Tool attributes
	BeadID     string            // ID of the bead this event belongs to (set by context wrapper)
}

// ProgressObserver receives progress updates from Core execution.
// All methods are optional — implement only what you need.
// Methods are called synchronously from the execution goroutine.
type ProgressObserver interface {
	// OnLoopStart is called when the loop begins.
	OnLoopStart(rootBead string)

	// OnBeadStart is called when work begins on a bead.
	OnBeadStart(bead beads.Bead)

	// OnBeadComplete is called when a bead finishes (success or failure).
	OnBeadComplete(result BeadResult)

	// OnLoopEnd is called when the loop completes.
	OnLoopEnd(result *CoreResult)

	// OnToolStart is called when a tool call starts.
	OnToolStart(event ToolEvent)

	// OnToolEnd is called when a tool call ends.
	OnToolEnd(event ToolEvent)
}

// NoopObserver is a ProgressObserver that does nothing.
// Embed this in your observer to avoid implementing unused methods.
type NoopObserver struct{}

func (NoopObserver) OnLoopStart(string)            {}
func (NoopObserver) OnBeadStart(beads.Bead)        {}
func (NoopObserver) OnBeadComplete(BeadResult)      {}
func (NoopObserver) OnLoopEnd(*CoreResult)         {}
func (NoopObserver) OnToolStart(ToolEvent)         {}
func (NoopObserver) OnToolEnd(ToolEvent)           {}

// beadContextObserver wraps an observer to tag tool events with a bead ID.
// This enables correct routing of events in parallel execution scenarios.
type beadContextObserver struct {
	inner  ProgressObserver
	beadID string
}

// newBeadContextObserver creates an observer wrapper that tags tool events with the bead ID.
func newBeadContextObserver(inner ProgressObserver, beadID string) ProgressObserver {
	if inner == nil {
		return nil
	}
	return &beadContextObserver{inner: inner, beadID: beadID}
}

func (o *beadContextObserver) OnLoopStart(rootBead string) {
	o.inner.OnLoopStart(rootBead)
}

func (o *beadContextObserver) OnBeadStart(bead beads.Bead) {
	o.inner.OnBeadStart(bead)
}

func (o *beadContextObserver) OnBeadComplete(result BeadResult) {
	o.inner.OnBeadComplete(result)
}

func (o *beadContextObserver) OnLoopEnd(result *CoreResult) {
	o.inner.OnLoopEnd(result)
}

func (o *beadContextObserver) OnToolStart(event ToolEvent) {
	event.BeadID = o.beadID
	o.inner.OnToolStart(event)
}

func (o *beadContextObserver) OnToolEnd(event ToolEvent) {
	event.BeadID = o.beadID
	o.inner.OnToolEnd(event)
}

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

	// Observer receives progress updates. Optional.
	Observer ProgressObserver

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

	// Notify observer of loop start
	if c.Observer != nil {
		c.Observer.OnLoopStart(c.RootBead)
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
		c.processBeadResults(ctx, wtMgr, results, result, out)
	}

	result.Duration = time.Since(start)

	// Print summary
	c.logSummary(result, out)

	// Notify observer of loop end
	if c.Observer != nil {
		c.Observer.OnLoopEnd(result)
	}

	return result, nil
}

// processBeadResults processes execution results, updates counters, merges worktrees, and cleans up.
func (c *Core) processBeadResults(ctx context.Context, wtMgr *WorktreeManager, results []beadExecResult, result *CoreResult, out io.Writer) {
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
		if r.WorktreePath != "" && r.Outcome == OutcomeSuccess && r.BranchName != "" {
			// Create worktree manager on-demand if we don't have one
			// This handles cases where MaxParallel was 1 but worktrees were created anyway
			mergeWtMgr := wtMgr
			if mergeWtMgr == nil {
				var err error
				mergeWtMgr, err = NewWorktreeManager(c.WorkDir)
				if err != nil {
					writef(out, "[%s] ERROR: failed to create worktree manager for merge: %v\n", r.BeadID, err)
					result.Failed++
					continue
				}
			}
			writef(out, "[%s] merging %s into %s\n", r.BeadID, r.BranchName, mergeWtMgr.Branch())
			if err := c.mergeBack(ctx, mergeWtMgr, r); err != nil {
				writef(out, "[%s] ERROR: merge failed: %v\n", r.BeadID, err)
				// Don't fail the entire run, but make the error visible
				result.Failed++
			} else {
				writef(out, "[%s] ✓ merged successfully\n", r.BeadID)
			}
		} else if r.BranchName != "" && r.Outcome == OutcomeSuccess {
			// Branch was created but worktree wasn't (shouldn't happen, but handle it)
			writef(out, "[%s] WARNING: branch %s exists but no worktree was created - merge skipped\n", r.BeadID, r.BranchName)
		}

		// Clean up worktree
		if r.WorktreePath != "" {
			cleanupWtMgr := wtMgr
			if cleanupWtMgr == nil {
				var err error
				cleanupWtMgr, err = NewWorktreeManager(c.WorkDir)
				if err != nil {
					writef(out, "  warning: failed to create worktree manager for cleanup: %v\n", err)
					continue
				}
			}
			if err := cleanupWtMgr.RemoveWorktree(r.WorktreePath); err != nil {
				writef(out, "  warning: failed to remove worktree: %v\n", err)
			}
		}
	}
}

// logSummary logs the completion summary to the output writer.
func (c *Core) logSummary(result *CoreResult, out io.Writer) {
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
	writef(out, "  Duration: %s\n", FormatDuration(result.Duration))
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
// If RootBead is set, returns ready children of that bead.
// If no children are found, checks if RootBead itself is ready (for single-bead targeting).
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

	ready, err := parseReadyBeads(out)
	if err != nil {
		return nil, err
	}

	// If we have children, return them
	if len(ready) > 0 {
		return ready, nil
	}

	// No children - check if RootBead itself is ready (single-bead targeting)
	if c.RootBead != "" {
		return c.getBeadIfReady(runner, c.RootBead)
	}

	return nil, nil
}

// getBeadIfReady returns the bead as a single-item slice if it's ready to work on.
// A bead is ready if status is "open" and it has no blocking dependencies.
func (c *Core) getBeadIfReady(runner BDRunner, beadID string) ([]beads.Bead, error) {
	out, err := runner(c.WorkDir, "show", beadID, "--json")
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", beadID, err)
	}

	var entries []struct {
		ID              string    `json:"id"`
		Title           string    `json:"title"`
		Description     string    `json:"description"`
		Status          string    `json:"status"`
		Priority        int       `json:"priority"`
		Labels          []string  `json:"labels"`
		CreatedAt       time.Time `json:"created_at"`
		IssueType       string    `json:"issue_type"`
		DependencyCount int       `json:"dependency_count"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}

	e := entries[0]

	// Bead is ready if open and has no blockers
	if e.Status != "open" || e.DependencyCount > 0 {
		return nil, nil
	}

	return []beads.Bead{{
		ID:        e.ID,
		Title:     e.Title,
		Status:    e.Status,
		Priority:  e.Priority,
		Labels:    e.Labels,
		CreatedAt: e.CreatedAt,
	}}, nil
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

	// For observer notifications
	var agentResult *AgentResult
	notifyComplete := func(outcome Outcome, errMsg string) {
		if c.Observer != nil {
			br := BeadResult{
				Bead:     *bead,
				Outcome:  outcome,
				Duration: result.Duration,
			}
			if agentResult != nil {
				br.ChatID = agentResult.ChatID
				br.ExitCode = agentResult.ExitCode
				br.Stderr = agentResult.Stderr
			}
			if errMsg != "" {
				br.ErrorMessage = errMsg
			}
			c.Observer.OnBeadComplete(br)
		}
	}

	// Notify observer of bead start
	if c.Observer != nil {
		c.Observer.OnBeadStart(*bead)
	}

	// Determine execution directory
	execDir := c.WorkDir
	if wtMgr != nil {
		worktreePath, branchName, err := wtMgr.CreateWorktree(bead.ID)
		if err != nil {
			writef(out, "[%s] failed to create worktree: %v\n", bead.ID, err)
			result.Outcome = OutcomeFailure
			result.Duration = time.Since(start)
			notifyComplete(result.Outcome, err.Error())
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
		notifyComplete(result.Outcome, err.Error())
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
		notifyComplete(result.Outcome, err.Error())
		return result
	}

	// Execute agent
	if c.Execute != nil {
		agentResult, err = c.Execute(ctx, execDir, prompt)
	} else {
		var opts []Option
		if c.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(c.AgentTimeout))
		}
		if c.Observer != nil {
			// Wrap observer with bead context so tool events can be routed correctly
			// in parallel execution scenarios
			beadObserver := newBeadContextObserver(c.Observer, bead.ID)
			opts = append(opts, WithObserver(beadObserver))
			// Suppress stdout when observer is present (TUI mode) to avoid
			// interfering with bubbletea. The observer handles tool events,
			// and stdout is still captured in the buffer for parsing.
			opts = append(opts, WithStdoutWriter(io.Discard))
		}
		agentResult, err = RunAgent(ctx, execDir, prompt, opts...)
	}
	if err != nil {
		writef(out, "[%s] agent execution error: %v\n", bead.ID, err)
		result.Outcome = OutcomeFailure
		result.Duration = time.Since(start)
		notifyComplete(result.Outcome, err.Error())
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

	writef(out, "[%s] %s → %s (%s)\n", bead.ID, bead.Title, outcome, FormatDuration(result.Duration))
	if summary != "" && outcome != OutcomeSuccess {
		writef(out, "  %s\n", summary)
	}

	notifyComplete(result.Outcome, summary)
	return result
}

// mergeBack merges a worktree branch back into the main branch.
func (c *Core) mergeBack(ctx context.Context, wtMgr *WorktreeManager, r beadExecResult) error {
	// Find the correct repository path for merging.
	// Use the worktree that has the target branch checked out, not the main repo.
	mergeRepo := wtMgr.MergeRepo(wtMgr.Branch())
	return MergeWithAgentResolution(
		ctx,
		mergeRepo,
		wtMgr.Branch(),
		r.BranchName,
		r.BeadID,
		"", // beadTitle not needed
		c.AgentTimeout,
	)
}

