package ralph

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"devdeploy/internal/beads"
)

// concurrentLoopSetup holds setup state for concurrent loop.
type concurrentLoopSetup struct {
	out                 io.Writer
	summary             *RunSummary
	wtMgr               *WorktreeManager
	pickNext            func() (*beads.Bead, error)
	fetchPrompt         func(beadID string) (*PromptData, error)
	render              func(data *PromptData) (string, error)
	assessFn            func(beadID string, result *AgentResult) (Outcome, string)
	syncFn              func() error
	consecutiveLimit    int
	mu                  sync.Mutex
	consecutiveFailures int32
	lastFailedBeadID    string
	skippedBeads        map[string]bool
	iterations          int32
	stopReason          StopReason
	shouldStop          int32 // atomic flag for early termination
}

// setupConcurrentLoop initializes the concurrent loop setup.
func setupConcurrentLoop(ctx context.Context, cfg LoopConfig) (context.Context, *concurrentLoopSetup, func(), error) {
	resolved := resolveConfig(cfg)

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, resolved.wallTimeout)

	// Initialize worktree manager
	wtMgr, err := NewWorktreeManager(cfg.WorkDir)
	if err != nil {
		cancelWall()
		return nil, nil, nil, fmt.Errorf("creating worktree manager: %w", err)
	}

	// Set up picker
	pickNext := cfg.PickNext
	if pickNext == nil {
		picker := &BeadPicker{
			WorkDir: cfg.WorkDir,
			Epic:    cfg.Epic,
		}
		pickNext = picker.Next
	}

	// Shared state protected by mutex
	summary := &RunSummary{}
	consecutiveFailures := int32(0)
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)
	iterations := int32(0)
	stopReason := StopNormal
	shouldStop := int32(0) // atomic flag for early termination

	setup := &concurrentLoopSetup{
		out:                 resolved.out,
		summary:             summary,
		wtMgr:               wtMgr,
		pickNext:            pickNext,
		fetchPrompt:         resolved.fetchPrompt,
		render:              resolved.render,
		assessFn:            resolved.assessFn,
		syncFn:              resolved.syncFn,
		consecutiveLimit:    resolved.consecutiveLimit,
		mu:                  sync.Mutex{}, // Initialize as zero value, don't copy
		consecutiveFailures: consecutiveFailures,
		lastFailedBeadID:    lastFailedBeadID,
		skippedBeads:        skippedBeads,
		iterations:          iterations,
		stopReason:          stopReason,
		shouldStop:          shouldStop,
	}

	cleanup := func() {
		cancelWall()
	}

	return ctx, setup, cleanup, nil
}

// checkWorkerStopConditions checks if worker should stop due to context cancellation or max iterations.
func checkWorkerStopConditions(ctx context.Context, cfg LoopConfig, setup *concurrentLoopSetup) bool {
	// Check context cancellation
	if ctx.Err() != nil {
		setup.mu.Lock()
		if ctx.Err() == context.DeadlineExceeded {
			setup.stopReason = StopWallClock
		} else {
			setup.stopReason = StopContextCancelled
		}
		atomic.StoreInt32(&setup.shouldStop, 1)
		setup.mu.Unlock()
		return true
	}

	// Check max iterations
	if int(atomic.LoadInt32(&setup.iterations)) >= cfg.MaxIterations {
		setup.mu.Lock()
		if setup.stopReason == StopNormal {
			setup.stopReason = StopMaxIterations
		}
		atomic.StoreInt32(&setup.shouldStop, 1)
		setup.mu.Unlock()
		return true
	}

	return false
}

// pickAndValidateBead picks next bead and validates it (checks for skipping).
func pickAndValidateBead(setup *concurrentLoopSetup, cfg LoopConfig) (*beads.Bead, bool, error) {
	// Pick next bead (thread-safe)
	bead, err := setup.pickNext()
	if err != nil {
		setup.mu.Lock()
		atomic.StoreInt32(&setup.shouldStop, 1)
		setup.mu.Unlock()
		return nil, false, err
	}
	if bead == nil {
		setup.mu.Lock()
		if setup.stopReason == StopNormal {
			setup.stopReason = StopNormal
		}
		atomic.StoreInt32(&setup.shouldStop, 1)
		setup.mu.Unlock()
		return nil, false, nil
	}

	// Check if bead should be skipped
	setup.mu.Lock()
	if setup.skippedBeads[bead.ID] {
		setup.mu.Unlock()
		return nil, true, nil // continue loop
	}
	// Skip if this is the same bead that just failed
	if setup.lastFailedBeadID != "" && bead.ID == setup.lastFailedBeadID {
		setup.skippedBeads[bead.ID] = true
		setup.summary.Skipped++
		setup.lastFailedBeadID = ""
		setup.mu.Unlock()
		return nil, true, nil // continue loop
	}
	setup.mu.Unlock()

	return bead, false, nil
}

// executeAgentInWorktree creates worktree, fetches prompt, renders it, and executes agent.
func executeAgentInWorktree(ctx context.Context, cfg LoopConfig, setup *concurrentLoopSetup, workerID int, bead *beads.Bead, worktreePath, branchName string) (*AgentResult, error) {
	// Fetch prompt data (beads state is shared, so use original workdir)
	promptData, err := setup.fetchPrompt(bead.ID)
	if err != nil {
		setup.mu.Lock()
		writef(setup.out, "[worker %d] failed to fetch prompt for %s: %v\n", workerID, bead.ID, err)
		setup.mu.Unlock()
		return nil, err
	}

	// Render prompt
	prompt, err := setup.render(promptData)
	if err != nil {
		setup.mu.Lock()
		writef(setup.out, "[worker %d] failed to render prompt for %s: %v\n", workerID, bead.ID, err)
		setup.mu.Unlock()
		return nil, err
	}

	// Execute agent in worktree (use worktree path for isolation)
	agentExecute := func(ctx context.Context, prompt string) (*AgentResult, error) {
		var opts []Option
		if cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(cfg.AgentTimeout))
		}
		return RunAgent(ctx, worktreePath, prompt, opts...)
	}
	result, err := agentExecute(ctx, prompt)
	if err != nil {
		setup.mu.Lock()
		writef(setup.out, "[worker %d] failed to run agent for %s: %v\n", workerID, bead.ID, err)
		setup.mu.Unlock()
		return nil, err
	}

	return result, nil
}

// logWorkerIteration logs iteration results and verbose output.
func logWorkerIteration(setup *concurrentLoopSetup, cfg LoopConfig, iterNum int, bead *beads.Bead, outcome Outcome, result *AgentResult, outcomeSummary string) {
	// Print structured per-iteration log line
	writef(setup.out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))
	// Note: Bead summary not printed in concurrent mode (no formatter tracking per worker)

	// Verbose mode output
	if cfg.Verbose {
		printVerboseOutput(setup.out, result)
	}
}

// updateWorkerCounters updates counters based on outcome and checks consecutive failure limit.
func updateWorkerCounters(setup *concurrentLoopSetup, bead *beads.Bead, outcome Outcome) {
	// Update counters
	switch outcome {
	case OutcomeSuccess:
		setup.summary.Succeeded++
		atomic.StoreInt32(&setup.consecutiveFailures, 0)
		setup.lastFailedBeadID = ""
	case OutcomeQuestion:
		setup.summary.Questions++
		atomic.StoreInt32(&setup.consecutiveFailures, 0)
		setup.lastFailedBeadID = ""
	case OutcomeFailure:
		setup.summary.Failed++
		atomic.AddInt32(&setup.consecutiveFailures, 1)
		setup.lastFailedBeadID = bead.ID
	case OutcomeTimeout:
		setup.summary.TimedOut++
		atomic.AddInt32(&setup.consecutiveFailures, 1)
		setup.lastFailedBeadID = bead.ID
	}

	// Check consecutive failure limit
	if int(atomic.LoadInt32(&setup.consecutiveFailures)) >= setup.consecutiveLimit {
		setup.stopReason = StopConsecutiveFails
		atomic.StoreInt32(&setup.shouldStop, 1)
	}
}

// createConcurrentWorker creates a worker function for concurrent execution.
func createConcurrentWorker(ctx context.Context, cfg LoopConfig, setup *concurrentLoopSetup) func(workerID int) {
	return func(workerID int) {
		for atomic.LoadInt32(&setup.shouldStop) == 0 {
			// Check stop conditions
			if checkWorkerStopConditions(ctx, cfg, setup) {
				return
			}

			// Pick and validate bead
			bead, shouldContinue, err := pickAndValidateBead(setup, cfg)
			if err != nil {
				return
			}
			if shouldContinue {
				continue
			}
			if bead == nil {
				return
			}

			// Dry-run: print what would be done without executing
			if cfg.DryRun {
				setup.mu.Lock()
				iterNum := int(atomic.AddInt32(&setup.iterations, 1))
				writef(setup.out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
				setup.summary.Iterations++
				atomic.StoreInt32(&setup.shouldStop, 1)
				setup.mu.Unlock()
				return
			}

			// Create worktree for this bead
			worktreePath, branchName, err := setup.wtMgr.CreateWorktree(bead.ID)
			if err != nil {
				setup.mu.Lock()
				writef(setup.out, "[worker %d] failed to create worktree for %s: %v\n", workerID, bead.ID, err)
				setup.mu.Unlock()
				continue
			}
			defer func() {
				if err := setup.wtMgr.RemoveWorktree(worktreePath, branchName); err != nil {
					setup.mu.Lock()
					writef(setup.out, "[worker %d] warning: failed to remove worktree %s: %v\n", workerID, worktreePath, err)
					setup.mu.Unlock()
				}
			}()

			// Execute agent in worktree
			result, err := executeAgentInWorktree(ctx, cfg, setup, workerID, bead, worktreePath, branchName)
			if err != nil {
				continue
			}

			// Assess outcome (beads state is shared, so use original workdir)
			outcome, outcomeSummary := setup.assessFn(bead.ID, result)

			// Update shared state atomically
			setup.mu.Lock()
			iterNum := int(atomic.AddInt32(&setup.iterations, 1))
			setup.summary.Iterations++

			// Log iteration results
			logWorkerIteration(setup, cfg, iterNum, bead, outcome, result, outcomeSummary)

			// Update counters and check consecutive failures
			updateWorkerCounters(setup, bead, outcome)

			// Sync beads state (best-effort, don't fail on error)
			setup.mu.Unlock()
			if err := setup.syncFn(); err != nil {
				setup.mu.Lock()
				if cfg.Verbose {
					writef(setup.out, "  bd sync warning: %v\n", err)
				}
				setup.mu.Unlock()
			}
		}
	}
}

// runConcurrent executes the concurrent loop with worker pool pattern.
func runConcurrent(ctx context.Context, cfg LoopConfig, concurrency int) (*RunSummary, error) {
	ctx, setup, cleanup, err := setupConcurrentLoop(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loopStart := time.Now()

	// Create worker function
	worker := createConcurrentWorker(ctx, cfg, setup)

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			worker(id)
		}(i)
	}

	// Wait for all workers to finish
	wg.Wait()

	// Calculate total duration
	setup.summary.Duration = time.Since(loopStart)
	setup.summary.StopReason = setup.stopReason

	// Count remaining beads
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary
	writef(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))

	return setup.summary, nil
}
