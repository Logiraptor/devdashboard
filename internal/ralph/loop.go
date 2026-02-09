package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"devdeploy/internal/beads"
)

// StopReason indicates why the ralph loop terminated.
type StopReason int

const (
	StopNormal           StopReason = iota // No more beads or all iterations completed.
	StopMaxIterations                      // Hit --max-iterations cap.
	StopConsecutiveFails                   // Too many consecutive failures.
	StopWallClock                          // Total --timeout wall-clock exceeded.
	StopContextCancelled                   // Context cancelled (e.g. SIGINT).
	StopAllBeadsSkipped                    // All available beads were skipped (retry detection).
)

// String returns a human-readable label for the stop reason.
func (r StopReason) String() string {
	switch r {
	case StopNormal:
		return "normal"
	case StopMaxIterations:
		return "max-iterations"
	case StopConsecutiveFails:
		return "consecutive-failures"
	case StopWallClock:
		return "wall-clock-timeout"
	case StopContextCancelled:
		return "context-cancelled"
	case StopAllBeadsSkipped:
		return "all-beads-skipped"
	default:
		return "unknown"
	}
}

// ExitCode returns a distinct process exit code for each stop reason.
func (r StopReason) ExitCode() int {
	switch r {
	case StopNormal:
		return 0
	case StopMaxIterations:
		return 2
	case StopConsecutiveFails:
		return 3
	case StopWallClock:
		return 4
	case StopContextCancelled:
		return 5
	case StopAllBeadsSkipped:
		return 6
	default:
		return 1
	}
}

// DefaultConsecutiveFailureLimit is the default number of consecutive failures
// before the loop stops.
const DefaultConsecutiveFailureLimit = 3

// DefaultWallClockTimeout is the default total wall-clock timeout for a ralph session.
const DefaultWallClockTimeout = 2 * time.Hour

// LoopConfig configures the ralph autonomous work loop.
type LoopConfig struct {
	WorkDir       string
	Epic          string
	Labels        []string
	TargetBead    string // if set, skip picker and work on this specific bead
	MaxIterations int
	DryRun        bool
	Verbose       bool

	// AgentTimeout is the per-agent execution timeout. Zero means use the
	// executor's DefaultTimeout (10m).
	AgentTimeout time.Duration

	// ConsecutiveFailureLimit stops the loop after N consecutive failures.
	// Zero means use DefaultConsecutiveFailureLimit (3).
	ConsecutiveFailureLimit int

	// Timeout is the total wall-clock timeout for the entire ralph session.
	// Zero means use DefaultWallClockTimeout (2h).
	Timeout time.Duration

	// Concurrency is the number of concurrent agents to run. Default is 1 (sequential).
	// When > 1, each agent runs in its own git worktree for isolation.
	Concurrency int

	// StrictLanding, when true, treats incomplete landing (uncommitted changes or
	// unclosed bead) as failure. When false, warns but counts as success if bead closed.
	// Default is true.
	StrictLanding bool

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
	Skipped    int
	StopReason StopReason
	Duration   time.Duration
}

// formatDuration formats a duration in a human-readable way (e.g., "2m34s", "1h12m").
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatIterationLog formats a per-iteration log line.
func formatIterationLog(iter, maxIter int, beadID, title string, outcome Outcome, duration time.Duration, outcomeSummary string) string {
	var status string
	switch outcome {
	case OutcomeSuccess:
		status = "success"
	case OutcomeQuestion:
		// Extract question bead IDs from outcomeSummary: "bead X has N question(s) needing human input: id1, id2"
		status = "question"
		if strings.Contains(outcomeSummary, ": ") {
			parts := strings.Split(outcomeSummary, ": ")
			if len(parts) > 1 {
				questionIDs := strings.TrimSpace(parts[1])
				status = fmt.Sprintf("question: %s", questionIDs)
			}
		}
	case OutcomeFailure:
		status = "failed"
		// Extract exit code from: "bead X still open after agent run (exit code N, duration ...)"
		// or: "failed to query bead X: ... (agent exit code N)"
		if strings.Contains(outcomeSummary, "exit code") {
			// Find "exit code" and extract the number after it
			idx := strings.Index(outcomeSummary, "exit code")
			if idx >= 0 {
				afterCode := outcomeSummary[idx+len("exit code"):]
				afterCode = strings.TrimSpace(afterCode)
				// Extract first number
				var exitCode string
				for _, r := range afterCode {
					if r >= '0' && r <= '9' {
						exitCode += string(r)
					} else if exitCode != "" {
						break
					}
				}
				if exitCode != "" {
					status = fmt.Sprintf("failed: exit code %s", exitCode)
				}
			}
		}
	case OutcomeTimeout:
		status = "timeout"
		// Extract exit code from: "agent timed out after ... (exit code N)"
		if strings.Contains(outcomeSummary, "exit code") {
			idx := strings.Index(outcomeSummary, "exit code")
			if idx >= 0 {
				afterCode := outcomeSummary[idx+len("exit code"):]
				afterCode = strings.TrimSpace(afterCode)
				// Extract first number
				var exitCode string
				for _, r := range afterCode {
					if r >= '0' && r <= '9' {
						exitCode += string(r)
					} else if exitCode != "" {
						break
					}
				}
				if exitCode != "" {
					status = fmt.Sprintf("timeout: exit code %s", exitCode)
				}
			}
		}
		// For timeout, also show the timeout duration if available
		if strings.Contains(outcomeSummary, "timed out after") {
			idx := strings.Index(outcomeSummary, "timed out after")
			if idx >= 0 {
				afterAfter := outcomeSummary[idx+len("timed out after"):]
				afterAfter = strings.TrimSpace(afterAfter)
				// Extract duration (everything up to " (")
				if parenIdx := strings.Index(afterAfter, " ("); parenIdx >= 0 {
					timeoutDur := strings.TrimSpace(afterAfter[:parenIdx])
					status = fmt.Sprintf("timeout (%s)", timeoutDur)
				}
			}
		}
	}

	return fmt.Sprintf("[%d/%d] %s \"%s\" → %s (%s)",
		iter, maxIter, beadID, title, status, formatDuration(duration))
}

// formatSummary formats the end-of-loop summary.
func formatSummary(summary *RunSummary, remainingBeads int) string {
	var lines []string
	lines = append(lines, "Ralph loop complete:")

	if summary.Succeeded > 0 {
		lines = append(lines, fmt.Sprintf("  ✓ %d beads completed", summary.Succeeded))
	}
	if summary.Questions > 0 {
		lines = append(lines, fmt.Sprintf("  ? %d questions created (needs human)", summary.Questions))
	}
	if summary.Failed > 0 {
		lines = append(lines, fmt.Sprintf("  ✗ %d failure(s)", summary.Failed))
	}
	if summary.TimedOut > 0 {
		lines = append(lines, fmt.Sprintf("  ⏱ %d timeout(s)", summary.TimedOut))
	}
	if summary.Skipped > 0 {
		lines = append(lines, fmt.Sprintf("  ⊘ %d skipped", summary.Skipped))
	}
	if remainingBeads > 0 {
		lines = append(lines, fmt.Sprintf("  ○ %d beads remaining (blocked)", remainingBeads))
	}

	lines = append(lines, fmt.Sprintf("  Duration: %s", formatDuration(summary.Duration)))

	return strings.Join(lines, "\n")
}

// countRemainingBeads counts the number of ready beads remaining.
func countRemainingBeads(cfg LoopConfig) int {
	if cfg.PickNext == nil {
		picker := &BeadPicker{
			WorkDir: cfg.WorkDir,
			Epic:    cfg.Epic,
			Labels:  cfg.Labels,
		}
		count, err := picker.Count()
		if err != nil {
			return 0
		}
		return count
	}

	// With a custom picker, we can't easily count
	// Try to pick one to see if any remain
	bead, err := cfg.PickNext()
	if err != nil || bead == nil {
		return 0
	}
	return 1 // At least one remains
}

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
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Use sequential path for concurrency=1 to maintain exact current behavior
	if concurrency == 1 {
		return runSequential(ctx, cfg)
	}

	// Use concurrent path for concurrency > 1
	return runConcurrent(ctx, cfg, concurrency)
}

// runEpicOrchestrator orchestrates epic leaf tasks sequentially, then runs opus verification.
// This is invoked when --epic flag is set (and --bead is not set).
// It queries 'bd ready --parent <epic-id>' after each completion to get fresh state
// and handle newly ready leaves, continuing until all leaves are done.
func runEpicOrchestrator(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	loopStart := time.Now()

	// Resolve defaults.
	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, wallTimeout)
	defer cancelWall()

	// Set up status writer for devdeploy TUI polling.
	statusWriter := NewStatusWriter(cfg.WorkDir)
	defer statusWriter.Clear()

	summary := &RunSummary{}

	// Fetch epic info for verification prompt
	epicPromptData, err := FetchPromptData(nil, cfg.WorkDir, cfg.Epic)
	if err != nil {
		return nil, fmt.Errorf("fetching epic prompt data: %w", err)
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
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			if !cfg.Verbose {
				formatter := NewLogFormatter(out, false)
				opts = append(opts, WithStdoutWriter(formatter))
			} else if out != os.Stdout {
				opts = append(opts, WithStdoutWriter(out))
			}
			return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
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

	// Track processed beads to avoid infinite loops if a bead keeps appearing
	processedBeads := make(map[string]bool)

	// Fetch children function - can be overridden for testing
	fetchChildren := func() ([]beads.Bead, error) {
		return FetchEpicChildren(nil, cfg.WorkDir, cfg.Epic, cfg.Labels)
	}

	// Main loop: query for ready leaves, process first one, repeat until none remain
	iteration := 0
	for {
		// Guard: context cancellation / wall-clock timeout.
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				summary.StopReason = StopWallClock
			} else {
				summary.StopReason = StopContextCancelled
			}
			break
		}

		// Query for ready leaf tasks
		if iteration == 0 {
			fmt.Fprintf(out, "Epic orchestrator: querying 'bd ready --parent %s' for leaf tasks\n", cfg.Epic)
		}
		children, err := fetchChildren()
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("fetching epic children: %w", err)
		}

		// Filter out already processed beads
		readyChildren := make([]beads.Bead, 0, len(children))
		for _, child := range children {
			if !processedBeads[child.ID] {
				readyChildren = append(readyChildren, child)
			}
		}

		if len(readyChildren) == 0 {
			if iteration == 0 {
				fmt.Fprintf(out, "No ready children found for epic %s\n", cfg.Epic)
			} else {
				fmt.Fprintf(out, "No more ready children for epic %s\n", cfg.Epic)
			}
			summary.StopReason = StopNormal
			break
		}

		if iteration == 0 {
			fmt.Fprintf(out, "Found %d ready leaf task(s) for epic %s\n", len(readyChildren), cfg.Epic)
		}

		// Process the first ready leaf (sorted by priority)
		child := readyChildren[0]
		iteration++

		// Mark as processed to avoid reprocessing
		processedBeads[child.ID] = true

		// Write status: starting iteration with current bead.
		currentBead := &BeadInfo{ID: child.ID, Title: child.Title}
		status := Status{
			State:         "running",
			Iteration:     iteration,
			MaxIterations: 0, // Unknown total, will be updated
			CurrentBead:   currentBead,
			Elapsed:       time.Since(loopStart).Nanoseconds(),
		}
		status.Tallies.Completed = summary.Succeeded
		status.Tallies.Questions = summary.Questions
		status.Tallies.Failed = summary.Failed
		status.Tallies.TimedOut = summary.TimedOut
		status.Tallies.Skipped = summary.Skipped
		_ = statusWriter.Write(status)

		// Fetch prompt data and render prompt
		promptData, err := fetchPrompt(child.ID)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: fetching prompt data for %s: %w", iteration, child.ID, err)
		}

		prompt, err := render(promptData)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: rendering prompt for %s: %w", iteration, child.ID, err)
		}

		// Get commit hash before agent execution for landing check
		cmd := exec.Command("git", "log", "-1", "--format=%H")
		cmd.Dir = cfg.WorkDir
		commitHashBefore := ""
		if outBytes, err := cmd.Output(); err == nil {
			commitHashBefore = strings.TrimSpace(string(outBytes))
		}

		// Execute agent
		result, err := execute(ctx, prompt)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: running agent for %s: %w", iteration, child.ID, err)
		}

		// Assess outcome
		outcome, outcomeSummary := assessFn(child.ID, result)

		// Check landing status
		landingStatus, landingErr := CheckLanding(cfg.WorkDir, child.ID, commitHashBefore)
		if landingErr == nil {
			landingMsg := FormatLandingStatus(landingStatus)
			if landingMsg != "landed successfully" {
				fmt.Fprintf(out, "  Landing: %s\n", landingMsg)
				if cfg.StrictLanding && outcome == OutcomeSuccess {
					if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
						outcome = OutcomeFailure
						outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
					}
				}
			} else {
				fmt.Fprintf(out, "  Landing: %s\n", landingMsg)
			}
		}

		// Print structured per-iteration log line
		fmt.Fprintf(out, "%s\n", formatIterationLog(iteration, 0, child.ID, child.Title, outcome, result.Duration, outcomeSummary))

		// Update counters
		summary.Iterations++
		switch outcome {
		case OutcomeSuccess:
			summary.Succeeded++
		case OutcomeQuestion:
			summary.Questions++
		case OutcomeFailure:
			summary.Failed++
			// If a child fails, stop the epic orchestration
			summary.StopReason = StopConsecutiveFails
			summary.Duration = time.Since(loopStart)
			return summary, nil
		case OutcomeTimeout:
			summary.TimedOut++
			// If a child times out, stop the epic orchestration
			summary.StopReason = StopWallClock
			summary.Duration = time.Since(loopStart)
			return summary, nil
		}

		// Sync beads state after each completion
		if err := syncFn(); err != nil {
			if cfg.Verbose {
				fmt.Fprintf(out, "  bd sync warning: %v\n", err)
			}
		}

		// Continue loop to query for next ready leaf
	}

	// If all children completed successfully, run opus verification
	if summary.Failed == 0 && summary.TimedOut == 0 && summary.Iterations > 0 {
		fmt.Fprintf(out, "\nAll %d leaf task(s) completed successfully. Running opus verification...\n", summary.Iterations)

		// Create verification prompt
		verificationPrompt := fmt.Sprintf(`You are verifying epic %s after all leaf tasks have been completed.

# %s

All child tasks have been completed. Please review the work and verify:
1. All requirements from the epic description have been met
2. The implementation is consistent and complete
3. Code quality and testing standards are met
4. Documentation is updated if needed

If everything looks good, you can close the epic with: bd close %s

If issues are found, create question beads or update the epic description.`, cfg.Epic, epicPromptData.Title, cfg.Epic)

		// Run opus verification
		var opts []Option
		if cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(cfg.AgentTimeout))
		}
		if !cfg.Verbose {
			formatter := NewLogFormatter(out, false)
			opts = append(opts, WithStdoutWriter(formatter))
		} else if out != os.Stdout {
			opts = append(opts, WithStdoutWriter(out))
		}

		opusResult, err := RunAgentOpus(ctx, cfg.WorkDir, verificationPrompt, opts...)
		if err != nil {
			fmt.Fprintf(out, "Opus verification failed to run: %v\n", err)
		} else {
			fmt.Fprintf(out, "\nOpus verification completed (exit code %d, duration %s)\n", opusResult.ExitCode, formatDuration(opusResult.Duration))
		}
	}

	summary.Duration = time.Since(loopStart)
	if summary.StopReason == StopNormal && summary.Failed == 0 && summary.TimedOut == 0 {
		summary.StopReason = StopNormal
	}

	// Write final status
	finalStatus := Status{
		State:         "completed",
		Iteration:     summary.Iterations,
		MaxIterations: summary.Iterations, // Total equals completed since we process until none remain
		Elapsed:       summary.Duration.Nanoseconds(),
		StopReason:    summary.StopReason.String(),
	}
	finalStatus.Tallies.Completed = summary.Succeeded
	finalStatus.Tallies.Questions = summary.Questions
	finalStatus.Tallies.Failed = summary.Failed
	finalStatus.Tallies.TimedOut = summary.TimedOut
	finalStatus.Tallies.Skipped = summary.Skipped
	_ = statusWriter.Write(finalStatus)

	// Count remaining beads for summary
	remainingBeads := 0
	if summary.StopReason == StopNormal {
		// Query one more time to see if any remain
		remaining, err := fetchChildren()
		if err == nil {
			remainingBeads = len(remaining)
		}
	}

	// Print final summary
	fmt.Fprintf(out, "\n%s\n", formatSummary(summary, remainingBeads))

	return summary, nil
}

// runSequential executes the sequential loop (original implementation).
func runSequential(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Epic mode: when Epic is set and TargetBead is empty, orchestrate children sequentially
	if cfg.Epic != "" && cfg.TargetBead == "" {
		return runEpicOrchestrator(ctx, cfg)
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	loopStart := time.Now()

	// Resolve defaults.
	consecutiveLimit := cfg.ConsecutiveFailureLimit
	if consecutiveLimit <= 0 {
		consecutiveLimit = DefaultConsecutiveFailureLimit
	}

	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, wallTimeout)
	defer cancelWall()

	pickNext := cfg.PickNext
	if pickNext == nil {
		if cfg.TargetBead != "" {
			// When TargetBead is set, create a picker that always returns that bead
			// (only on first call, then returns nil)
			once := false
			pickNext = func() (*beads.Bead, error) {
				if once {
					return nil, nil // Signal no more beads
				}
				once = true
				// Use bd list to get full bead info (includes all fields we need)
				cmd := exec.Command("bd", "list", "--json", "--limit", "1")
				cmd.Dir = cfg.WorkDir
				// Filter by ID using grep-like approach, or use bd show and construct minimal Bead
				// Actually, let's use bd list with a filter, but bd list doesn't filter by ID directly
				// So we'll use bd show to verify it exists and get title, then construct Bead
				showCmd := exec.Command("bd", "show", cfg.TargetBead, "--json")
				showCmd.Dir = cfg.WorkDir
				out, err := showCmd.Output()
				if err != nil {
					return nil, fmt.Errorf("bd show %s: %w", cfg.TargetBead, err)
				}
				// bd show returns an array with one entry containing id, title, description
				var showEntries []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				}
				if err := json.Unmarshal(out, &showEntries); err != nil {
					return nil, fmt.Errorf("parsing bd show output: %w", err)
				}
				if len(showEntries) == 0 {
					return nil, fmt.Errorf("bead %s not found", cfg.TargetBead)
				}
				e := showEntries[0]
				// Construct a minimal Bead - other fields will have defaults
				// Status, Priority, Labels, CreatedAt will be empty/zero, which is fine
				// for our purposes since we just need ID and Title for the prompt
				return &beads.Bead{
					ID:    e.ID,
					Title: e.Title,
					// Default values for other fields are fine
				}, nil
			}
		} else {
			picker := &BeadPicker{
				WorkDir: cfg.WorkDir,
				Epic:    cfg.Epic,
				Labels:  cfg.Labels,
			}
			pickNext = picker.Next
		}
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
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			// Wrap stdout with log formatter if not verbose
			if !cfg.Verbose {
				formatter := NewLogFormatter(out, false)
				opts = append(opts, WithStdoutWriter(formatter))
			} else if out != os.Stdout {
				opts = append(opts, WithStdoutWriter(out))
			}
			return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
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
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)

	// Set up status writer for devdeploy TUI polling.
	statusWriter := NewStatusWriter(cfg.WorkDir)
	defer statusWriter.Clear() // Clean up status file when done

	for i := 0; i < cfg.MaxIterations; i++ {
		// Guard: context cancellation / wall-clock timeout.
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				summary.StopReason = StopWallClock
			} else {
				summary.StopReason = StopContextCancelled
			}
			// Write final status before breaking
			status := Status{
				State:         "completed",
				Iteration:     summary.Iterations,
				MaxIterations: cfg.MaxIterations,
				Elapsed:       time.Since(loopStart).Nanoseconds(),
				StopReason:    summary.StopReason.String(),
			}
			status.Tallies.Completed = summary.Succeeded
			status.Tallies.Questions = summary.Questions
			status.Tallies.Failed = summary.Failed
			status.Tallies.TimedOut = summary.TimedOut
			status.Tallies.Skipped = summary.Skipped
			_ = statusWriter.Write(status)
			break
		}

		// 1. Pick next bead.
		bead, err := pickNext()
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: picking bead: %w", i+1, err)
		}
		if bead == nil {
			summary.StopReason = StopNormal
			// Write final status before breaking
			status := Status{
				State:         "completed",
				Iteration:     summary.Iterations,
				MaxIterations: cfg.MaxIterations,
				Elapsed:       time.Since(loopStart).Nanoseconds(),
				StopReason:    summary.StopReason.String(),
			}
			status.Tallies.Completed = summary.Succeeded
			status.Tallies.Questions = summary.Questions
			status.Tallies.Failed = summary.Failed
			status.Tallies.TimedOut = summary.TimedOut
			status.Tallies.Skipped = summary.Skipped
			_ = statusWriter.Write(status)
			break
		}

		// Write status: starting iteration with current bead.
		currentBead := &BeadInfo{ID: bead.ID, Title: bead.Title}
		status := Status{
			State:         "running",
			Iteration:     i + 1,
			MaxIterations: cfg.MaxIterations,
			CurrentBead:   currentBead,
			Elapsed:       time.Since(loopStart).Nanoseconds(),
		}
		status.Tallies.Completed = summary.Succeeded
		status.Tallies.Questions = summary.Questions
		status.Tallies.Failed = summary.Failed
		status.Tallies.TimedOut = summary.TimedOut
		status.Tallies.Skipped = summary.Skipped
		_ = statusWriter.Write(status) // Best effort; don't fail loop on status write errors

		// Guard: same-bead retry detection.
		// If the same bead that just failed is picked again, skip it.
		if lastFailedBeadID != "" && bead.ID == lastFailedBeadID {
			skippedBeads[bead.ID] = true
			summary.Skipped++
			lastFailedBeadID = "" // reset so we don't skip indefinitely

			// Check if we should stop: if we've skipped beads and the picker
			// keeps returning skipped beads, we're in an infinite loop.
			// Try one more pick; if that's also skipped, stop.
			retryBead, retryErr := pickNext()
			if retryErr != nil {
				summary.Duration = time.Since(loopStart)
				return summary, fmt.Errorf("iteration %d: picking bead (retry): %w", i+1, retryErr)
			}
			if retryBead == nil {
				summary.StopReason = StopNormal
				break
			}
			if skippedBeads[retryBead.ID] {
				summary.Skipped++
				summary.StopReason = StopAllBeadsSkipped
				// Write final status before breaking
				status := Status{
					State:         "completed",
					Iteration:     summary.Iterations,
					MaxIterations: cfg.MaxIterations,
					Elapsed:       time.Since(loopStart).Nanoseconds(),
					StopReason:    summary.StopReason.String(),
				}
				status.Tallies.Completed = summary.Succeeded
				status.Tallies.Questions = summary.Questions
				status.Tallies.Failed = summary.Failed
				status.Tallies.TimedOut = summary.TimedOut
				status.Tallies.Skipped = summary.Skipped
				_ = statusWriter.Write(status)
				break
			}
			bead = retryBead
		}

		// Dry-run: print what would be done without executing.
		if cfg.DryRun {
			fmt.Fprintf(out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
			summary.Iterations++
			// Write final status for dry-run
			status := Status{
				State:         "completed",
				Iteration:     summary.Iterations,
				MaxIterations: cfg.MaxIterations,
				Elapsed:       time.Since(loopStart).Nanoseconds(),
				StopReason:    "dry-run",
			}
			status.Tallies.Completed = summary.Succeeded
			status.Tallies.Questions = summary.Questions
			status.Tallies.Failed = summary.Failed
			status.Tallies.TimedOut = summary.TimedOut
			status.Tallies.Skipped = summary.Skipped
			_ = statusWriter.Write(status)
			break
		}

		// 2. Fetch full bead data and render prompt.
		promptData, err := fetchPrompt(bead.ID)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: fetching prompt data for %s: %w",
				i+1, bead.ID, err)
		}

		prompt, err := render(promptData)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: rendering prompt for %s: %w",
				i+1, bead.ID, err)
		}

		// 3. Get commit hash before agent execution for landing check.
		cmd := exec.Command("git", "log", "-1", "--format=%H")
		cmd.Dir = cfg.WorkDir
		commitHashBefore := ""
		if out, err := cmd.Output(); err == nil {
			commitHashBefore = strings.TrimSpace(string(out))
		}

		// 4. Execute agent.
		result, err := execute(ctx, prompt)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: running agent for %s: %w",
				i+1, bead.ID, err)
		}

		// 5. Assess outcome.
		outcome, outcomeSummary := assessFn(bead.ID, result)

		// 6. Check landing status.
		landingStatus, landingErr := CheckLanding(cfg.WorkDir, bead.ID, commitHashBefore)
		if landingErr == nil {
			landingMsg := FormatLandingStatus(landingStatus)
			if landingMsg != "landed successfully" {
				fmt.Fprintf(out, "  Landing: %s\n", landingMsg)
				// If strict landing is enabled and landing is incomplete, treat as failure
				if cfg.StrictLanding && outcome == OutcomeSuccess {
					// Override success if landing is incomplete
					if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
						outcome = OutcomeFailure
						outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
					}
				}
			} else {
				fmt.Fprintf(out, "  Landing: %s\n", landingMsg)
			}
		}

		// Print structured per-iteration log line.
		fmt.Fprintf(out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))

		// Verbose mode: print agent stdout/stderr excerpts.
		if cfg.Verbose {
			if result.Stdout != "" {
				lines := strings.Split(result.Stdout, "\n")
				maxLines := 10
				if len(lines) > maxLines {
					fmt.Fprintf(out, "  stdout (showing last %d lines):\n", maxLines)
					for _, line := range lines[len(lines)-maxLines:] {
						fmt.Fprintf(out, "    %s\n", line)
					}
				} else {
					fmt.Fprintf(out, "  stdout:\n")
					for _, line := range lines {
						if line != "" {
							fmt.Fprintf(out, "    %s\n", line)
						}
					}
				}
			}
			if result.Stderr != "" {
				lines := strings.Split(result.Stderr, "\n")
				maxLines := 10
				if len(lines) > maxLines {
					fmt.Fprintf(out, "  stderr (showing last %d lines):\n", maxLines)
					for _, line := range lines[len(lines)-maxLines:] {
						fmt.Fprintf(out, "    %s\n", line)
					}
				} else {
					fmt.Fprintf(out, "  stderr:\n")
					for _, line := range lines {
						if line != "" {
							fmt.Fprintf(out, "    %s\n", line)
						}
					}
				}
			}
		}

		// 5. Update counters.
		summary.Iterations++
		switch outcome {
		case OutcomeSuccess:
			summary.Succeeded++
			consecutiveFailures = 0
			lastFailedBeadID = ""
		case OutcomeQuestion:
			summary.Questions++
			consecutiveFailures = 0
			lastFailedBeadID = ""
		case OutcomeFailure:
			summary.Failed++
			consecutiveFailures++
			lastFailedBeadID = bead.ID
		case OutcomeTimeout:
			summary.TimedOut++
			consecutiveFailures++
			lastFailedBeadID = bead.ID
		}

		// Write status update after iteration completes.
		status = Status{
			State:         "running",
			Iteration:     i + 1,
			MaxIterations: cfg.MaxIterations,
			Elapsed:       time.Since(loopStart).Nanoseconds(),
		}
		status.Tallies.Completed = summary.Succeeded
		status.Tallies.Questions = summary.Questions
		status.Tallies.Failed = summary.Failed
		status.Tallies.TimedOut = summary.TimedOut
		status.Tallies.Skipped = summary.Skipped
		_ = statusWriter.Write(status) // Best effort

		// Guard: consecutive failure limit.
		if consecutiveFailures >= consecutiveLimit {
			summary.StopReason = StopConsecutiveFails
			// Write final status before breaking
			status.State = "completed"
			status.StopReason = summary.StopReason.String()
			_ = statusWriter.Write(status)
			break
		}

		// 6. Sync beads state.
		if err := syncFn(); err != nil {
			if cfg.Verbose {
				fmt.Fprintf(out, "  bd sync warning: %v\n", err)
			}
		}
	}

	// If we exhausted all iterations without an earlier stop reason, set it.
	if summary.StopReason == StopNormal && summary.Iterations >= cfg.MaxIterations {
		summary.StopReason = StopMaxIterations
	}

	// Calculate total duration.
	summary.Duration = time.Since(loopStart)

	// Write final status.
	finalStatus := Status{
		State:         "completed",
		Iteration:     summary.Iterations,
		MaxIterations: cfg.MaxIterations,
		Elapsed:       summary.Duration.Nanoseconds(),
		StopReason:    summary.StopReason.String(),
	}
	finalStatus.Tallies.Completed = summary.Succeeded
	finalStatus.Tallies.Questions = summary.Questions
	finalStatus.Tallies.Failed = summary.Failed
	finalStatus.Tallies.TimedOut = summary.TimedOut
	finalStatus.Tallies.Skipped = summary.Skipped
	_ = statusWriter.Write(finalStatus)

	// Count remaining beads.
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary (always printed, even on early termination).
	fmt.Fprintf(out, "\n%s\n", formatSummary(summary, remainingBeads))

	return summary, nil
}

// runConcurrent executes the concurrent loop with worker pool pattern.
func runConcurrent(ctx context.Context, cfg LoopConfig, concurrency int) (*RunSummary, error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}
	loopStart := time.Now()

	// Resolve defaults.
	consecutiveLimit := cfg.ConsecutiveFailureLimit
	if consecutiveLimit <= 0 {
		consecutiveLimit = DefaultConsecutiveFailureLimit
	}

	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, wallTimeout)
	defer cancelWall()

	// Initialize worktree manager
	wtMgr, err := NewWorktreeManager(cfg.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("creating worktree manager: %w", err)
	}

	// Set up picker
	pickNext := cfg.PickNext
	if pickNext == nil {
		picker := &BeadPicker{
			WorkDir: cfg.WorkDir,
			Epic:    cfg.Epic,
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
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
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

	// Shared state protected by mutex
	var mu sync.Mutex
	summary := &RunSummary{}
	consecutiveFailures := int32(0)
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)
	iterations := int32(0)
	stopReason := StopNormal
	shouldStop := int32(0) // atomic flag for early termination

	// Worker function
	worker := func(workerID int) {
		for atomic.LoadInt32(&shouldStop) == 0 {
			// Check context cancellation
			if ctx.Err() != nil {
				mu.Lock()
				if ctx.Err() == context.DeadlineExceeded {
					stopReason = StopWallClock
				} else {
					stopReason = StopContextCancelled
				}
				atomic.StoreInt32(&shouldStop, 1)
				mu.Unlock()
				return
			}

			// Check max iterations
			if int(atomic.LoadInt32(&iterations)) >= cfg.MaxIterations {
				mu.Lock()
				if stopReason == StopNormal {
					stopReason = StopMaxIterations
				}
				atomic.StoreInt32(&shouldStop, 1)
				mu.Unlock()
				return
			}

			// Pick next bead (thread-safe)
			bead, err := pickNext()
			if err != nil {
				mu.Lock()
				atomic.StoreInt32(&shouldStop, 1)
				mu.Unlock()
				return
			}
			if bead == nil {
				mu.Lock()
				if stopReason == StopNormal {
					stopReason = StopNormal
				}
				atomic.StoreInt32(&shouldStop, 1)
				mu.Unlock()
				return
			}

			// Check if bead should be skipped
			mu.Lock()
			if skippedBeads[bead.ID] {
				mu.Unlock()
				continue
			}
			// Skip if this is the same bead that just failed
			if lastFailedBeadID != "" && bead.ID == lastFailedBeadID {
				skippedBeads[bead.ID] = true
				summary.Skipped++
				lastFailedBeadID = ""
				mu.Unlock()
				continue
			}
			mu.Unlock()

			// Dry-run: print what would be done without executing
			if cfg.DryRun {
				mu.Lock()
				iterNum := int(atomic.AddInt32(&iterations, 1))
				fmt.Fprintf(out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
				summary.Iterations++
				atomic.StoreInt32(&shouldStop, 1)
				mu.Unlock()
				return
			}

			// Create worktree for this bead
			worktreePath, branchName, err := wtMgr.CreateWorktree(bead.ID)
			if err != nil {
				mu.Lock()
				fmt.Fprintf(out, "[worker %d] failed to create worktree for %s: %v\n", workerID, bead.ID, err)
				mu.Unlock()
				continue
			}
			defer func() {
				if err := wtMgr.RemoveWorktree(worktreePath, branchName); err != nil {
					mu.Lock()
					fmt.Fprintf(out, "[worker %d] warning: failed to remove worktree %s: %v\n", workerID, worktreePath, err)
					mu.Unlock()
				}
			}()

			// Fetch prompt data (beads state is shared, so use original workdir)
			promptData, err := fetchPrompt(bead.ID)
			if err != nil {
				mu.Lock()
				fmt.Fprintf(out, "[worker %d] failed to fetch prompt for %s: %v\n", workerID, bead.ID, err)
				mu.Unlock()
				continue
			}

			// Render prompt
			prompt, err := render(promptData)
			if err != nil {
				mu.Lock()
				fmt.Fprintf(out, "[worker %d] failed to render prompt for %s: %v\n", workerID, bead.ID, err)
				mu.Unlock()
				continue
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
				mu.Lock()
				fmt.Fprintf(out, "[worker %d] failed to run agent for %s: %v\n", workerID, bead.ID, err)
				mu.Unlock()
				continue
			}

			// Assess outcome (beads state is shared, so use original workdir)
			outcome, outcomeSummary := assessFn(bead.ID, result)

			// Update shared state atomically
			mu.Lock()
			iterNum := int(atomic.AddInt32(&iterations, 1))
			summary.Iterations++

			// Print structured per-iteration log line
			fmt.Fprintf(out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))

			// Verbose mode output
			if cfg.Verbose {
				if result.Stdout != "" {
					lines := strings.Split(result.Stdout, "\n")
					maxLines := 10
					if len(lines) > maxLines {
						fmt.Fprintf(out, "  stdout (showing last %d lines):\n", maxLines)
						for _, line := range lines[len(lines)-maxLines:] {
							fmt.Fprintf(out, "    %s\n", line)
						}
					} else {
						fmt.Fprintf(out, "  stdout:\n")
						for _, line := range lines {
							if line != "" {
								fmt.Fprintf(out, "    %s\n", line)
							}
						}
					}
				}
				if result.Stderr != "" {
					lines := strings.Split(result.Stderr, "\n")
					maxLines := 10
					if len(lines) > maxLines {
						fmt.Fprintf(out, "  stderr (showing last %d lines):\n", maxLines)
						for _, line := range lines[len(lines)-maxLines:] {
							fmt.Fprintf(out, "    %s\n", line)
						}
					} else {
						fmt.Fprintf(out, "  stderr:\n")
						for _, line := range lines {
							if line != "" {
								fmt.Fprintf(out, "    %s\n", line)
							}
						}
					}
				}
			}

			// Update counters
			switch outcome {
			case OutcomeSuccess:
				summary.Succeeded++
				atomic.StoreInt32(&consecutiveFailures, 0)
				lastFailedBeadID = ""
			case OutcomeQuestion:
				summary.Questions++
				atomic.StoreInt32(&consecutiveFailures, 0)
				lastFailedBeadID = ""
			case OutcomeFailure:
				summary.Failed++
				atomic.AddInt32(&consecutiveFailures, 1)
				lastFailedBeadID = bead.ID
			case OutcomeTimeout:
				summary.TimedOut++
				atomic.AddInt32(&consecutiveFailures, 1)
				lastFailedBeadID = bead.ID
			}

			// Check consecutive failure limit
			if int(atomic.LoadInt32(&consecutiveFailures)) >= consecutiveLimit {
				stopReason = StopConsecutiveFails
				atomic.StoreInt32(&shouldStop, 1)
			}

			// Sync beads state (best-effort, don't fail on error)
			mu.Unlock()
			if err := syncFn(); err != nil {
				mu.Lock()
				if cfg.Verbose {
					fmt.Fprintf(out, "  bd sync warning: %v\n", err)
				}
				mu.Unlock()
			}
		}
	}

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
	summary.Duration = time.Since(loopStart)
	summary.StopReason = stopReason

	// Count remaining beads
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary
	fmt.Fprintf(out, "\n%s\n", formatSummary(summary, remainingBeads))

	return summary, nil
}
