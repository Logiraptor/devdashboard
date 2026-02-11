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
			return Assess(cfg.WorkDir, beadID, result)
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

// executeBead executes a single bead in its own worktree.
func (w *WaveOrchestrator) executeBead(ctx context.Context, bead *beads.Bead) {
	// Create worktree for this bead
	worktreePath, branchName, err := w.setup.wtMgr.CreateWorktree(bead.ID)
	if err != nil {
		w.setup.mu.Lock()
		_, _ = fmt.Fprintf(w.setup.out, "[wave] failed to create worktree for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		return
	}
	defer func() {
		if err := w.setup.wtMgr.RemoveWorktree(worktreePath, branchName); err != nil {
			w.setup.mu.Lock()
			_, _ = fmt.Fprintf(w.setup.out, "[wave] warning: failed to remove worktree %s: %v\n", worktreePath, err)
			w.setup.mu.Unlock()
		}
	}()

	// Fetch prompt data
	promptData, err := w.setup.fetchPrompt(bead.ID)
	if err != nil {
		w.setup.mu.Lock()
		_, _ = fmt.Fprintf(w.setup.out, "[wave] failed to fetch prompt for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		return
	}

	// Render prompt
	prompt, err := w.setup.render(promptData)
	if err != nil {
		w.setup.mu.Lock()
		_, _ = fmt.Fprintf(w.setup.out, "[wave] failed to render prompt for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		return
	}

	// Execute agent in worktree
	agentExecute := func(ctx context.Context, prompt string) (*AgentResult, error) {
		var opts []Option
		if w.cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(w.cfg.AgentTimeout))
		}
		return RunAgent(ctx, worktreePath, prompt, opts...)
	}

	result, err := agentExecute(ctx, prompt)
	if err != nil {
		w.setup.mu.Lock()
		_, _ = fmt.Fprintf(w.setup.out, "[wave] failed to run agent for %s: %v\n", bead.ID, err)
		w.setup.mu.Unlock()
		return
	}

	// Assess outcome
	outcome, outcomeSummary := w.setup.assessFn(bead.ID, result)

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
	_, _ = fmt.Fprintf(w.setup.out, "%s\n", formatIterationLog(
		w.setup.summary.Iterations,
		w.cfg.MaxIterations,
		bead.ID,
		bead.Title,
		outcome,
		result.Duration,
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
			_, _ = fmt.Fprintf(w.setup.out, "  bd sync warning: %v\n", err)
		}
		w.setup.mu.Unlock()
	}
}

// Run dispatches all ready beads in parallel and waits for completion.
func (w *WaveOrchestrator) Run(ctx context.Context) (*RunSummary, error) {
	// Apply wall-clock timeout if configured
	wallTimeout := w.cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallTimeout)
	defer cancel()

	waveStart := time.Now()

	// Fetch all ready beads
	readyBeads, err := w.fetchReadyBeads()
	if err != nil {
		return nil, fmt.Errorf("fetching ready beads: %w", err)
	}

	if len(readyBeads) == 0 {
		w.setup.summary.StopReason = StopNormal
		w.setup.summary.Duration = time.Since(waveStart)
		_, _ = fmt.Fprintf(w.setup.out, "No ready beads found\n")
		return w.setup.summary, nil
	}

	// Dry-run: print what would be done without executing
	if w.cfg.DryRun {
		for i, bead := range readyBeads {
			_, _ = fmt.Fprintf(w.setup.out, "%s\n", formatIterationLog(
				i+1,
				len(readyBeads),
				bead.ID,
				bead.Title,
				OutcomeSuccess,
				0,
				"",
			))
		}
		w.setup.summary.Iterations = len(readyBeads)
		w.setup.summary.StopReason = StopNormal
		w.setup.summary.Duration = time.Since(waveStart)
		return w.setup.summary, nil
	}

	_, _ = fmt.Fprintf(w.setup.out, "Wave orchestrator: dispatching %d ready bead(s) in parallel\n", len(readyBeads))

	// Dispatch all beads in parallel
	var wg sync.WaitGroup
	for i := range readyBeads {
		wg.Add(1)
		go func(bead *beads.Bead) {
			defer wg.Done()
			w.executeBead(ctx, bead)
		}(&readyBeads[i])
	}

	// Wait for all beads to complete
	wg.Wait()

	// Calculate total duration
	w.setup.summary.Duration = time.Since(waveStart)
	w.setup.summary.StopReason = StopNormal

	// Count remaining beads
	remainingBeads := countRemainingBeads(w.cfg)

	// Print final summary
	_, _ = fmt.Fprintf(w.setup.out, "\n%s\n", formatSummary(w.setup.summary, remainingBeads))

	return w.setup.summary, nil
}
