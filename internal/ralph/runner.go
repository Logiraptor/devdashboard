package ralph

import (
	"context"
	"fmt"
	"iter"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"devdeploy/internal/beads"
)

// BeadBatcher yields batches of beads to process.
// Each batch runs in parallel; the iterator controls parallelism.
type BeadBatcher = iter.Seq[[]beads.Bead]

// Runner executes beads in parallel batches using an iter.Seq batcher.
type Runner struct {
	batcher     BeadBatcher
	cfg         LoopConfig
	summary     *RunSummary
	mu          sync.Mutex
	wtMgr       *WorktreeManager
	fetchPrompt func(beadID string) (*PromptData, error)
	render      func(data *PromptData) (string, error)
	execute     func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn    func(beadID string, result *AgentResult) (Outcome, string)
	out         io.Writer
	consecutiveFailures int
	consecutiveLimit    int
	lastFailedBeadID    string
}

// NewRunner creates a new Runner with the given batcher and configuration.
func NewRunner(batcher BeadBatcher, cfg LoopConfig) (*Runner, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	// Initialize worktree manager for parallel execution
	// Only create worktree manager if we might need it (not for single-bead sequential execution)
	// We'll create it lazily when needed
	var wtMgr *WorktreeManager

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

	consecutiveLimit := cfg.ConsecutiveFailureLimit
	if consecutiveLimit <= 0 {
		consecutiveLimit = DefaultConsecutiveFailureLimit
	}

	return &Runner{
		batcher:             batcher,
		cfg:                 cfg,
		summary:             &RunSummary{},
		wtMgr:               wtMgr,
		fetchPrompt:         fetchPrompt,
		render:              render,
		execute:             execute,
		assessFn:            assessFn,
		out:                 out,
		consecutiveFailures:  0,
		consecutiveLimit:     consecutiveLimit,
		lastFailedBeadID:    "",
	}, nil
}

// Run executes beads from the batcher in parallel batches.
// For each batch yielded by the iterator, it fans out to goroutines (one per bead).
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

	// Call OnBatchStart callback if provided
	if r.cfg.OnBatchStart != nil {
		// We'll call this when we get the first batch
	}

	for batch := range r.batcher {
		// Check stop conditions
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				r.summary.StopReason = StopWallClock
			} else {
				r.summary.StopReason = StopContextCancelled
			}
			break
		}

		// Check max iterations (limit total beads processed)
		if r.cfg.MaxIterations > 0 && r.summary.Iterations >= r.cfg.MaxIterations {
			r.summary.StopReason = StopMaxIterations
			break
		}

		if len(batch) == 0 {
			// Empty batch, skip it
			batchNum++
			continue
		}

		batchNum++

		// Call OnBatchStart callback if provided
		if r.cfg.OnBatchStart != nil {
			r.cfg.OnBatchStart(batch, batchNum)
		}

		// Dry-run: print what would be done without executing
		if r.cfg.DryRun {
			results := make([]BeadResult, 0, len(batch))
			for i, bead := range batch {
				writef(r.out, "%s\n", formatIterationLog(
					r.summary.Iterations+i+1,
					r.cfg.MaxIterations,
					bead.ID,
					bead.Title,
					OutcomeSuccess,
					0,
					"",
				))
				results = append(results, BeadResult{
					Bead:     bead,
					Outcome:  OutcomeSuccess,
					Duration: 0,
				})
			}
			r.summary.Iterations += len(batch)
			// Call OnBatchEnd callback if provided
			if r.cfg.OnBatchEnd != nil {
				r.cfg.OnBatchEnd(batchNum, results)
			}
			continue
		}

		// Initialize worktree manager if needed for parallel execution
		if len(batch) > 1 && r.wtMgr == nil {
			wtMgr, err := NewWorktreeManager(r.cfg.WorkDir)
			if err != nil {
				return nil, fmt.Errorf("creating worktree manager: %w", err)
			}
			r.wtMgr = wtMgr
		}

		if batchNum == 1 && len(batch) > 1 {
			writef(r.out, "Batch %d: dispatching %d bead(s) in parallel\n", batchNum, len(batch))
		}

		// Fan out to goroutines (one per bead)
		var wg sync.WaitGroup
		results := make([]*BeadResult, len(batch))
		for i := range batch {
			wg.Add(1)
			go func(idx int, bead *beads.Bead) {
				defer wg.Done()
				results[idx] = r.executeBead(ctx, bead, len(batch) > 1)
			}(i, &batch[i])
		}

		// Wait for all beads in this batch to complete
		wg.Wait()

		// Collect results and check consecutive failures
		batchResults := make([]BeadResult, 0, len(results))
		r.mu.Lock()
		for _, result := range results {
			if result != nil {
				batchResults = append(batchResults, *result)
				// Track consecutive failures
				if result.Outcome == OutcomeFailure || result.Outcome == OutcomeTimeout {
					r.consecutiveFailures++
					r.lastFailedBeadID = result.Bead.ID
				} else {
					r.consecutiveFailures = 0
					r.lastFailedBeadID = ""
				}
			}
		}
		shouldStop := r.consecutiveFailures >= r.consecutiveLimit
		r.mu.Unlock()

		// Check consecutive failure limit
		if shouldStop {
			r.summary.StopReason = StopConsecutiveFails
			break
		}

		// Call OnBatchEnd callback if provided
		if r.cfg.OnBatchEnd != nil {
			r.cfg.OnBatchEnd(batchNum, batchResults)
		}
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
// Returns a BeadResult if execution completed, nil if it failed early.
// useWorktree indicates whether to use a worktree for isolation (for parallel execution).
func (r *Runner) executeBead(ctx context.Context, bead *beads.Bead, useWorktree bool) *BeadResult {
	// Call OnBeadStart callback if provided
	if r.cfg.OnBeadStart != nil {
		r.cfg.OnBeadStart(*bead)
	}

	beadStart := time.Now()

	// Create worktree if needed for parallel execution
	var worktreePath string
	var branchName string
	if useWorktree && r.wtMgr != nil {
		var err error
		worktreePath, branchName, err = r.wtMgr.CreateWorktree(bead.ID)
		if err != nil {
			r.mu.Lock()
			writef(r.out, "[runner] failed to create worktree for %s: %v\n", bead.ID, err)
			r.mu.Unlock()
			if r.cfg.OnBeadEnd != nil {
				r.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
			}
			return nil
		}
		defer func() {
			if removeErr := r.wtMgr.RemoveWorktree(worktreePath); removeErr != nil {
				r.mu.Lock()
				writef(r.out, "[runner] warning: failed to remove worktree %s: %v\n", worktreePath, removeErr)
				r.mu.Unlock()
			}
		}()
	}

	// Fetch prompt data (use original workdir for beads state)
	promptData, err := r.fetchPrompt(bead.ID)
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to fetch prompt for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		if r.cfg.OnBeadEnd != nil {
			r.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Render prompt
	prompt, err := r.render(promptData)
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to render prompt for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		if r.cfg.OnBeadEnd != nil {
			r.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Get commit hash before agent execution for landing check
	commitHashBefore := getCommitHashBefore(r.cfg.WorkDir)

	// Determine execution path: use worktreePath if provided, otherwise use main workdir
	execPath := r.cfg.WorkDir
	if worktreePath != "" {
		execPath = worktreePath
	}

	// Execute agent
	var result *AgentResult
	if r.cfg.Execute != nil {
		// Use custom execute function if provided (for testing)
		result, err = r.cfg.Execute(ctx, prompt)
	} else {
		// Use default execution with worktree path
		var opts []Option
		if r.cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(r.cfg.AgentTimeout))
		}
		result, err = RunAgent(ctx, execPath, prompt, opts...)
	}
	if err != nil {
		r.mu.Lock()
		writef(r.out, "[runner] failed to run agent for %s: %v\n", bead.ID, err)
		r.mu.Unlock()
		if r.cfg.OnBeadEnd != nil {
			r.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Assess outcome
	outcome, outcomeSummary := r.assessFn(bead.ID, result)
	duration := result.Duration

	// Merge successful work back into the original branch if using worktree
	if worktreePath != "" && outcome == OutcomeSuccess {
		// Merge worktree changes back to main branch
		if mergeErr := MergeWithAgentResolution(ctx, r.wtMgr.SrcRepo(), r.wtMgr.Branch(), branchName, bead.ID, bead.Title, r.cfg.AgentTimeout); mergeErr != nil {
			r.mu.Lock()
			writef(r.out, "[runner] warning: failed to merge %s back to %s: %v\n", branchName, r.wtMgr.Branch(), mergeErr)
			r.mu.Unlock()
			// Don't change outcome - the work was done, just the merge failed
		}
	}

	// Check landing status
	landingStatus, landingErr := CheckLanding(r.cfg.WorkDir, bead.ID, commitHashBefore)
	if landingErr == nil {
		landingMsg := FormatLandingStatus(landingStatus)
		if landingMsg != "landed successfully" {
			r.mu.Lock()
			writef(r.out, "  Landing: %s\n", landingMsg)
			r.mu.Unlock()
			// If strict landing is enabled and landing is incomplete, treat as failure
			if r.cfg.StrictLanding && outcome == OutcomeSuccess {
				// Override success if landing is incomplete
				if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
					outcome = OutcomeFailure
					outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
				}
			}
		} else {
			r.mu.Lock()
			writef(r.out, "  Landing: %s\n", landingMsg)
			r.mu.Unlock()
		}
	}

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
		duration,
		outcomeSummary,
	))

	// Verbose mode output
	if r.cfg.Verbose {
		printVerboseOutput(r.out, result)
	}

	r.mu.Unlock()

	// Sync beads state (best-effort, don't fail on error)
	if r.cfg.SyncFn != nil {
		if syncErr := r.cfg.SyncFn(); syncErr != nil {
			r.mu.Lock()
			if r.cfg.Verbose {
				writef(r.out, "  bd sync warning: %v\n", syncErr)
			}
			r.mu.Unlock()
		}
	}

	// Call OnBeadEnd callback if provided
	if r.cfg.OnBeadEnd != nil {
		r.cfg.OnBeadEnd(*bead, outcome, duration)
	}

	return &BeadResult{
		Bead:     *bead,
		Outcome:  outcome,
		Duration: duration,
	}
}

// getCommitHashBefore gets the commit hash before agent execution for landing check.
func getCommitHashBefore(workDir string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = workDir
	if outBytes, gitErr := cmd.Output(); gitErr == nil {
		return strings.TrimSpace(string(outBytes))
	}
	return ""
}
