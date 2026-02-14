package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"devdeploy/internal/beads"
)

// WaveOrchestrator dispatches all ready beads in parallel.
// Unlike the concurrent loop which picks beads one at a time as workers become available,
// WaveOrchestrator fetches all ready beads upfront and dispatches them all at once.
type WaveOrchestrator struct {
	cfg   LoopConfig
	setup *waveSetup
}

// waveSetup holds setup state for wave execution.
type waveSetup struct {
	out         io.Writer
	summary     *RunSummary
	wtMgr       *WorktreeManager
	fetchPrompt func(beadID string) (*PromptData, error)
	render      func(data *PromptData) (string, error)
	execute     func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn    func(beadID string, result *AgentResult) (Outcome, string)
	syncFn      func() error
	mu          sync.Mutex
}

// NewWaveOrchestrator creates a new WaveOrchestrator with the given configuration.
func NewWaveOrchestrator(cfg LoopConfig) (*WaveOrchestrator, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	// Initialize worktree manager
	wtMgr, err := NewWorktreeManager(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("creating worktree manager: %w", err)
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

	// Set up sync
	syncFn := cfg.SyncFn
	if syncFn == nil {
		syncFn = func() error {
			cmd := exec.Command("bd", "sync")
			cmd.Dir = cfg.WorkDir
			return cmd.Run()
		}
	}

	setup := &waveSetup{
		out:         out,
		summary:     &RunSummary{},
		wtMgr:       wtMgr,
		fetchPrompt: fetchPrompt,
		render:      render,
		execute:     execute,
		assessFn:    assessFn,
		syncFn:      syncFn,
	}

	return &WaveOrchestrator{
		cfg:   cfg,
		setup: setup,
	}, nil
}

// fetchReadyBeads fetches all ready beads.
// If Epic is set, fetches ready children of that epic.
// Otherwise, fetches all ready beads.
func (w *WaveOrchestrator) fetchReadyBeads() ([]beads.Bead, error) {
	if w.cfg.Epic != "" {
		// Fetch ready children of epic
		runBD := func(dir string, args ...string) ([]byte, error) {
			cmd := exec.Command("bd", args...)
			cmd.Dir = dir
			return cmd.Output()
		}
		return FetchEpicChildren(runBD, w.cfg.WorkDir, w.cfg.Epic)
	}

	// Fetch all ready beads (no epic filter)
	runBD := func(dir string, args ...string) ([]byte, error) {
		cmd := exec.Command("bd", args...)
		cmd.Dir = dir
		return cmd.Output()
	}

	args := []string{"ready", "--json"}

	out, err := runBD(w.cfg.WorkDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return parsed, nil
}

// executeBead executes a single bead, optionally in its own worktree.
// If worktreePath is empty, uses the main workdir (cfg.WorkDir).
// If worktreePath is set, cleans up the worktree after execution.
// Returns a BeadResult if execution completed, nil if it failed early.
func (w *WaveOrchestrator) executeBead(ctx context.Context, bead *beads.Bead, worktreePath string) *BeadResult {
	// Call OnBeadStart callback if provided
	if w.cfg.OnBeadStart != nil {
		w.cfg.OnBeadStart(*bead)
	}

	beadStart := time.Now()

	// If worktreePath is provided, set up cleanup
	if worktreePath != "" {
		defer func() {
			if err := w.setup.wtMgr.RemoveWorktree(worktreePath); err != nil {
				w.setup.mu.Lock()
				writef(w.setup.out, "[wave] warning: failed to remove worktree %s: %v\n", worktreePath, err)
				w.setup.mu.Unlock()
			}
		}()
	}

	// Fetch prompt data
	promptData, err := w.setup.fetchPrompt(bead.ID)
	if err != nil {
		w.setup.mu.Lock()
		writef(w.setup.out, "[wave] failed to fetch prompt for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		if w.cfg.OnBeadEnd != nil {
			w.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Render prompt
	prompt, err := w.setup.render(promptData)
	if err != nil {
		w.setup.mu.Lock()
		writef(w.setup.out, "[wave] failed to render prompt for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		if w.cfg.OnBeadEnd != nil {
			w.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Determine execution path: use worktreePath if provided, otherwise use main workdir
	execPath := worktreePath
	if execPath == "" {
		execPath = w.cfg.WorkDir
	}

	// Execute agent
	// If cfg.Execute is provided (typically for testing), use it directly
	// Otherwise, run the agent in the determined path
	var result *AgentResult
	if w.cfg.Execute != nil {
		result, err = w.cfg.Execute(ctx, prompt)
	} else {
		var opts []Option
		if w.cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(w.cfg.AgentTimeout))
		}
		result, err = RunAgent(ctx, execPath, prompt, opts...)
	}
	if err != nil {
		w.setup.mu.Lock()
		writef(w.setup.out, "[wave] failed to run agent for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		if w.cfg.OnBeadEnd != nil {
			w.cfg.OnBeadEnd(*bead, OutcomeFailure, time.Since(beadStart))
		}
		return nil
	}

	// Assess outcome
	outcome, outcomeSummary := w.setup.assessFn(bead.ID, result)
	duration := result.Duration

	// Update summary
	w.setup.mu.Lock()
	w.setup.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		w.setup.summary.Succeeded++
	case OutcomeQuestion:
		w.setup.summary.Questions++
	case OutcomeFailure:
		w.setup.summary.Failed++
	case OutcomeTimeout:
		w.setup.summary.TimedOut++
	}

	// Log iteration
	writef(w.setup.out, "%s\n", formatIterationLog(
		w.setup.summary.Iterations,
		w.cfg.MaxIterations,
		bead.ID,
		bead.Title,
		outcome,
		duration,
		outcomeSummary,
	))

	// Verbose mode output
	if w.cfg.Verbose {
		printVerboseOutput(w.setup.out, result)
	}

	w.setup.mu.Unlock()

	// Sync beads state (best-effort, don't fail on error)
	if err := w.setup.syncFn(); err != nil {
		w.setup.mu.Lock()
		if w.cfg.Verbose {
			writef(w.setup.out, "  bd sync warning: %v\n", err)
		}
		w.setup.mu.Unlock()
	}

	// Call OnBeadEnd callback if provided
	if w.cfg.OnBeadEnd != nil {
		w.cfg.OnBeadEnd(*bead, outcome, duration)
	}

	beadResult := &BeadResult{
		Bead:         *bead,
		Outcome:      outcome,
		Duration:     duration,
		ChatID:       result.ChatID,
		ErrorMessage: result.ErrorMessage,
		ExitCode:     result.ExitCode,
		Stderr:       result.Stderr,
	}

	// Call OnBeadComplete callback with full result (for TUI integration)
	if w.cfg.OnBeadComplete != nil {
		w.cfg.OnBeadComplete(beadResult)
	}

	return beadResult
}

// Run dispatches all ready beads in parallel waves until no more are available.
// After each wave completes, it re-queries for newly unblocked beads.
func (w *WaveOrchestrator) Run(ctx context.Context) (*RunSummary, error) {
	// Apply wall-clock timeout if configured
	wallTimeout := w.cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallTimeout)
	defer cancel()

	waveStart := time.Now()

	// Track processed beads to avoid reprocessing in case of sync delays
	processedBeads := make(map[string]bool)
	waveNum := 0

	for {
		// Check context cancellation
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				w.setup.summary.StopReason = StopWallClock
			} else {
				w.setup.summary.StopReason = StopContextCancelled
			}
			break
		}

		// Fetch all ready beads
		readyBeads, err := w.fetchReadyBeads()
		if err != nil {
			return nil, fmt.Errorf("fetching ready beads: %w", err)
		}

		// Filter out already processed beads
		unprocessedBeads := make([]beads.Bead, 0, len(readyBeads))
		for _, bead := range readyBeads {
			if !processedBeads[bead.ID] {
				unprocessedBeads = append(unprocessedBeads, bead)
			}
		}

		if len(unprocessedBeads) == 0 {
			if waveNum == 0 {
				writef(w.setup.out, "No ready beads found\n")
			} else {
				writef(w.setup.out, "\nNo more ready beads after wave %d\n", waveNum)
			}
			w.setup.summary.StopReason = StopNormal
			break
		}

		waveNum++

		// Call OnBatchStart callback if provided
		if w.cfg.OnBatchStart != nil {
			w.cfg.OnBatchStart(unprocessedBeads, waveNum)
		}

		// Dry-run: print what would be done without executing
		if w.cfg.DryRun {
			results := make([]BeadResult, 0, len(unprocessedBeads))
			for i, bead := range unprocessedBeads {
				writef(w.setup.out, "%s\n", formatIterationLog(
					w.setup.summary.Iterations+i+1,
					0, // Don't show max in dry-run
					bead.ID,
					bead.Title,
					OutcomeSuccess,
					0,
					"",
				))
				processedBeads[bead.ID] = true
				results = append(results, BeadResult{
					Bead:     bead,
					Outcome:  OutcomeSuccess,
					Duration: 0,
				})
			}
			w.setup.summary.Iterations += len(unprocessedBeads)
			// Call OnBatchEnd callback if provided
			if w.cfg.OnBatchEnd != nil {
				w.cfg.OnBatchEnd(waveNum, results)
			}
			continue
		}

		writef(w.setup.out, "Wave %d: dispatching %d ready bead(s) in parallel\n", waveNum, len(unprocessedBeads))

		// Mark beads as processed before dispatching
		for _, bead := range unprocessedBeads {
			processedBeads[bead.ID] = true
		}

		// Dispatch all beads in parallel
		var wg sync.WaitGroup
		results := make([]*BeadResult, len(unprocessedBeads))

		if len(unprocessedBeads) > 1 {
			// Multiple beads: create worktree per bead for isolation
			for i := range unprocessedBeads {
				wg.Add(1)
				go func(idx int, bead *beads.Bead) {
					defer wg.Done()
					// Create worktree for this bead
					worktreePath, _, err := w.setup.wtMgr.CreateWorktree(bead.ID)
					if err != nil {
						w.setup.mu.Lock()
						writef(w.setup.out, "[wave] failed to create worktree for %s: %v\n", bead.ID, err)
						w.setup.mu.Unlock()
						// Call OnBeadEnd with failure outcome
						if w.cfg.OnBeadEnd != nil {
							w.cfg.OnBeadEnd(*bead, OutcomeFailure, 0)
						}
						return
					}
					results[idx] = w.executeBead(ctx, bead, worktreePath)
				}(i, &unprocessedBeads[i])
			}
		} else {
			// Single bead: run in main workdir
			wg.Add(1)
			go func(idx int, bead *beads.Bead) {
				defer wg.Done()
				results[idx] = w.executeBead(ctx, bead, "")
			}(0, &unprocessedBeads[0])
		}

		// Wait for all beads in this wave to complete
		wg.Wait()

		// Collect results (filter out nil results from early failures)
		batchResults := make([]BeadResult, 0, len(results))
		for _, result := range results {
			if result != nil {
				batchResults = append(batchResults, *result)
			}
		}

		// Call OnBatchEnd callback if provided
		if w.cfg.OnBatchEnd != nil {
			w.cfg.OnBatchEnd(waveNum, batchResults)
		}
	}

	// Calculate total duration
	w.setup.summary.Duration = time.Since(waveStart)

	// Count remaining beads
	remainingBeads := countRemainingBeads(w.cfg)

	// Print final summary
	writef(w.setup.out, "\n%s\n", formatSummary(w.setup.summary, remainingBeads))

	return w.setup.summary, nil
}
