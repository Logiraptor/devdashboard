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

// epicOrchestratorSetup holds setup state for epic orchestrator.
type epicOrchestratorSetup struct {
	out              io.Writer
	statusWriter     *StatusWriter
	summary          *RunSummary
	epicPromptData   *PromptData
	fetchPrompt      func(beadID string) (*PromptData, error)
	render           func(data *PromptData) (string, error)
	execute          func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn         func(beadID string, result *AgentResult) (Outcome, string)
	syncFn           func() error
	currentFormatter *LogFormatter
	processedBeads   map[string]bool
	fetchChildren    func() ([]beads.Bead, error)
}

// setupEpicOrchestrator initializes the epic orchestrator setup.
func setupEpicOrchestrator(ctx context.Context, cfg LoopConfig) (context.Context, *epicOrchestratorSetup, func(), error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	// Resolve defaults.
	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, wallTimeout)

	// Set up status writer for devdeploy TUI polling.
	statusWriter := NewStatusWriter(cfg.WorkDir)

	summary := &RunSummary{}

	// Fetch epic info for verification prompt
	epicPromptData, err := FetchPromptData(nil, cfg.WorkDir, cfg.Epic)
	if err != nil {
		cancelWall()
		return nil, nil, nil, fmt.Errorf("fetching epic prompt data: %w", err)
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

	// Track formatter for summary output
	var currentFormatter *LogFormatter

	execute := cfg.Execute
	if execute == nil {
		execute = func(ctx context.Context, prompt string) (*AgentResult, error) {
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			if !cfg.Verbose {
				currentFormatter = NewLogFormatter(out, false)
				opts = append(opts, WithStdoutWriter(currentFormatter))
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
		return FetchEpicChildren(nil, cfg.WorkDir, cfg.Epic)
	}

	setup := &epicOrchestratorSetup{
		out:              out,
		statusWriter:     statusWriter,
		summary:          summary,
		epicPromptData:   epicPromptData,
		fetchPrompt:      fetchPrompt,
		render:           render,
		execute:          execute,
		assessFn:         assessFn,
		syncFn:           syncFn,
		currentFormatter: currentFormatter,
		processedBeads:   processedBeads,
		fetchChildren:    fetchChildren,
	}

	cleanup := func() {
		cancelWall()
		_ = statusWriter.Clear()
	}

	return ctx, setup, cleanup, nil
}

// processEpicIteration processes a single epic iteration.
func processEpicIteration(ctx context.Context, cfg LoopConfig, setup *epicOrchestratorSetup, loopStart time.Time, iteration int) (bool, error) {
	// Guard: context cancellation / wall-clock timeout.
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			setup.summary.StopReason = StopWallClock
		} else {
			setup.summary.StopReason = StopContextCancelled
		}
		return false, nil
	}

	// Query for ready leaf tasks
	if iteration == 0 {
		_, _ = fmt.Fprintf(setup.out, "Epic orchestrator: querying 'bd ready --parent %s' for leaf tasks\n", cfg.Epic)
	}
	children, err := setup.fetchChildren()
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("fetching epic children: %w", err)
	}

	// Filter out already processed beads
	readyChildren := make([]beads.Bead, 0, len(children))
	for _, child := range children {
		if !setup.processedBeads[child.ID] {
			readyChildren = append(readyChildren, child)
		}
	}

	if len(readyChildren) == 0 {
		if iteration == 0 {
			_, _ = fmt.Fprintf(setup.out, "No ready children found for epic %s\n", cfg.Epic)
		} else {
			_, _ = fmt.Fprintf(setup.out, "No more ready children for epic %s\n", cfg.Epic)
		}
		setup.summary.StopReason = StopNormal
		return false, nil
	}

	if iteration == 0 {
		_, _ = fmt.Fprintf(setup.out, "Found %d ready leaf task(s) for epic %s\n", len(readyChildren), cfg.Epic)
	}

	// Process the first ready leaf (sorted by priority)
	child := readyChildren[0]
	iterNum := iteration + 1

	// Mark as processed to avoid reprocessing
	setup.processedBeads[child.ID] = true

	// Write status: starting iteration with current bead.
	currentBead := &BeadInfo{ID: child.ID, Title: child.Title}
	status := Status{
		State:         "running",
		Iteration:     iterNum,
		MaxIterations: 0, // Unknown total, will be updated
		CurrentBead:   currentBead,
		Elapsed:       time.Since(loopStart).Nanoseconds(),
	}
	status.Tallies.Completed = setup.summary.Succeeded
	status.Tallies.Questions = setup.summary.Questions
	status.Tallies.Failed = setup.summary.Failed
	status.Tallies.TimedOut = setup.summary.TimedOut
	status.Tallies.Skipped = setup.summary.Skipped
	// Ignore write errors: status updates are best-effort notifications for TUI polling.
	// Loop execution continues even if status file cannot be written.
	_ = setup.statusWriter.Write(status)

	// Fetch prompt data and render prompt
	promptData, err := setup.fetchPrompt(child.ID)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: fetching prompt data for %s: %w", iterNum, child.ID, err)
	}

	prompt, err := setup.render(promptData)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: rendering prompt for %s: %w", iterNum, child.ID, err)
	}

	// Get commit hash before agent execution for landing check
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = cfg.WorkDir
	commitHashBefore := ""
	if outBytes, err := cmd.Output(); err == nil {
		commitHashBefore = strings.TrimSpace(string(outBytes))
	}

	// Set current bead for progress display
	if setup.currentFormatter != nil {
		setup.currentFormatter.SetCurrentBead(child.ID, child.Title)
	}

	// Execute agent
	result, err := setup.execute(ctx, prompt)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: running agent for %s: %w", iterNum, child.ID, err)
	}

	// Print summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		// Clear progress line before printing summary
		_, _ = fmt.Fprintf(setup.out, "\n")
		if summaryStr := setup.currentFormatter.Summary(); summaryStr != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summaryStr)
		}
		setup.currentFormatter = nil // Reset for next iteration
	}

	// Assess outcome
	outcome, outcomeSummary := setup.assessFn(child.ID, result)

	// Check landing status
	landingStatus, landingErr := CheckLanding(cfg.WorkDir, child.ID, commitHashBefore)
	if landingErr == nil {
		landingMsg := FormatLandingStatus(landingStatus)
		if landingMsg != "landed successfully" {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
			if cfg.StrictLanding && outcome == OutcomeSuccess {
				if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
					outcome = OutcomeFailure
					outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
				}
			}
		} else {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
		}
	}

	// Print structured per-iteration log line
	_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(iterNum, 0, child.ID, child.Title, outcome, result.Duration, outcomeSummary))
	// Print bead summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		if summary := setup.currentFormatter.BeadSummary(); summary != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summary)
		}
	}

	// Update counters
	setup.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		setup.summary.Succeeded++
	case OutcomeQuestion:
		setup.summary.Questions++
	case OutcomeFailure:
		setup.summary.Failed++
		// If a child fails, stop the epic orchestration
		setup.summary.StopReason = StopConsecutiveFails
		setup.summary.Duration = time.Since(loopStart)
		return false, nil
	case OutcomeTimeout:
		setup.summary.TimedOut++
		// If a child times out, stop the epic orchestration
		setup.summary.StopReason = StopWallClock
		setup.summary.Duration = time.Since(loopStart)
		return false, nil
	}

	// Sync beads state after each completion
	if err := setup.syncFn(); err != nil {
		if cfg.Verbose {
			_, _ = fmt.Fprintf(setup.out, "  bd sync warning: %v\n", err)
		}
	}

	return true, nil
}

// runOpusVerification runs opus verification after all epic children complete.
func runOpusVerification(ctx context.Context, cfg LoopConfig, setup *epicOrchestratorSetup) {
	if setup.summary.Failed == 0 && setup.summary.TimedOut == 0 && setup.summary.Iterations > 0 {
		_, _ = fmt.Fprintf(setup.out, "\nAll %d leaf task(s) completed successfully. Running opus verification...\n", setup.summary.Iterations)

		// Fetch all epic children (including closed) for review
		allChildren, err := FetchAllEpicChildren(nil, cfg.WorkDir, cfg.Epic)
		if err != nil {
			_, _ = fmt.Fprintf(setup.out, "Warning: failed to fetch all epic children for verification: %v\n", err)
			allChildren = nil
		}

		// Get git log to see code changes (last 50 commits, or since epic creation if we can determine it)
		var gitLog string
		cmd := exec.Command("git", "log", "--oneline", "-50", "--no-decorate")
		cmd.Dir = cfg.WorkDir
		if outBytes, err := cmd.Output(); err == nil {
			gitLog = strings.TrimSpace(string(outBytes))
		}

		// Get git diff stats to see what changed
		var gitDiffStats string
		cmd = exec.Command("git", "diff", "--stat", "HEAD~50..HEAD")
		cmd.Dir = cfg.WorkDir
		if outBytes, err := cmd.Output(); err == nil {
			gitDiffStats = strings.TrimSpace(string(outBytes))
		}

		// Build verification prompt with closed tasks and code changes
		var promptBuilder strings.Builder
		promptBuilder.WriteString(fmt.Sprintf("You are verifying epic %s after all leaf tasks have been completed.\n\n", cfg.Epic))
		promptBuilder.WriteString(fmt.Sprintf("# %s\n\n", setup.epicPromptData.Title))
		promptBuilder.WriteString(fmt.Sprintf("%s\n\n", setup.epicPromptData.Description))
		promptBuilder.WriteString("---\n\n")
		promptBuilder.WriteString("## Verification Task\n\n")
		promptBuilder.WriteString("Verify the epic is fully implemented by:\n")
		promptBuilder.WriteString("1. Reviewing all closed tasks under this epic\n")
		promptBuilder.WriteString("2. Reviewing code changes made during implementation\n")
		promptBuilder.WriteString("3. Checking that all requirements from the epic description have been met\n")
		promptBuilder.WriteString("4. Ensuring implementation is consistent and complete\n")
		promptBuilder.WriteString("5. Verifying code quality and testing standards are met\n")
		promptBuilder.WriteString("6. Checking if documentation needs updates\n\n")

		if len(allChildren) > 0 {
			promptBuilder.WriteString("## Closed Tasks\n\n")
			closedCount := 0
			for _, child := range allChildren {
				if child.Status == "closed" {
					promptBuilder.WriteString(fmt.Sprintf("- **%s**: %s\n", child.ID, child.Title))
					closedCount++
				}
			}
			if closedCount == 0 {
				promptBuilder.WriteString("(No closed tasks found - this may indicate an issue)\n")
			}
			promptBuilder.WriteString("\n")
			promptBuilder.WriteString("Review each closed task to understand what was implemented:\n")
			for _, child := range allChildren {
				if child.Status == "closed" {
					promptBuilder.WriteString(fmt.Sprintf("- Run `bd show %s` to see details of task \"%s\"\n", child.ID, child.Title))
				}
			}
			promptBuilder.WriteString("\n")
		}

		if gitLog != "" {
			promptBuilder.WriteString("## Recent Code Changes\n\n")
			promptBuilder.WriteString("Recent commit history:\n```\n")
			// Limit to first 30 lines to avoid prompt bloat
			lines := strings.Split(gitLog, "\n")
			if len(lines) > 30 {
				promptBuilder.WriteString(strings.Join(lines[:30], "\n"))
				promptBuilder.WriteString(fmt.Sprintf("\n... (%d more commits)\n", len(lines)-30))
			} else {
				promptBuilder.WriteString(gitLog)
			}
			promptBuilder.WriteString("\n```\n\n")
		}

		if gitDiffStats != "" {
			promptBuilder.WriteString("## Files Changed\n\n")
			promptBuilder.WriteString("```\n")
			promptBuilder.WriteString(gitDiffStats)
			promptBuilder.WriteString("\n```\n\n")
		}

		promptBuilder.WriteString("## Actions\n\n")
		promptBuilder.WriteString("After your review:\n\n")
		promptBuilder.WriteString(fmt.Sprintf("1. **If everything is complete and correct**: Close the epic with `bd close %s`\n\n", cfg.Epic))
		promptBuilder.WriteString("2. **If you find gaps or improvements needed**: Create follow-up beads for each issue:\n")
		promptBuilder.WriteString("   ```bash\n")
		promptBuilder.WriteString(fmt.Sprintf("   bd create \"<description of gap/improvement>\" --type task --parent %s\n", cfg.Epic))
		promptBuilder.WriteString("   ```\n")
		promptBuilder.WriteString("   Be specific about what needs to be done. Review the code changes and closed tasks to identify:\n")
		promptBuilder.WriteString("   - Missing functionality\n")
		promptBuilder.WriteString("   - Incomplete implementations\n")
		promptBuilder.WriteString("   - Code quality issues\n")
		promptBuilder.WriteString("   - Missing tests\n")
		promptBuilder.WriteString("   - Documentation gaps\n")
		promptBuilder.WriteString("   - Edge cases not handled\n\n")
		promptBuilder.WriteString("3. **If you have questions**: Create question beads:\n")
		promptBuilder.WriteString("   ```bash\n")
		promptBuilder.WriteString(fmt.Sprintf("   bd create \"Question: <your question>\" --type task --label needs-human --parent %s\n", cfg.Epic))
		promptBuilder.WriteString("   ```\n\n")
		promptBuilder.WriteString("**Important**: Do not close the epic if you create any follow-up beads. Only close it when you are confident the epic is fully implemented.\n")

		verificationPrompt := promptBuilder.String()

		// Run opus verification
		var opts []Option
		var verificationFormatter *LogFormatter
		if cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(cfg.AgentTimeout))
		}
		if !cfg.Verbose {
			verificationFormatter = NewLogFormatter(setup.out, false)
			opts = append(opts, WithStdoutWriter(verificationFormatter))
		} else if setup.out != os.Stdout {
			opts = append(opts, WithStdoutWriter(setup.out))
		}

		opusResult, err := RunAgentOpus(ctx, cfg.WorkDir, verificationPrompt, opts...)
		if err != nil {
			_, _ = fmt.Fprintf(setup.out, "Opus verification failed to run: %v\n", err)
		} else {
			// Print summary if formatter was used
			if verificationFormatter != nil && !cfg.Verbose {
				if summaryStr := verificationFormatter.Summary(); summaryStr != "" {
					_, _ = fmt.Fprintf(setup.out, "%s\n", summaryStr)
				}
			}
			_, _ = fmt.Fprintf(setup.out, "\nOpus verification completed (exit code %d, duration %s)\n", opusResult.ExitCode, formatDuration(opusResult.Duration))
		}
	}
}

// writeFinalEpicStatus writes the final status and summary for epic orchestrator.
func writeFinalEpicStatus(cfg LoopConfig, setup *epicOrchestratorSetup, loopStart time.Time) {
	setup.summary.Duration = time.Since(loopStart)
	if setup.summary.StopReason == StopNormal && setup.summary.Failed == 0 && setup.summary.TimedOut == 0 {
		setup.summary.StopReason = StopNormal
	}

	// Write final status
	finalStatus := Status{
		State:         "completed",
		Iteration:     setup.summary.Iterations,
		MaxIterations: setup.summary.Iterations, // Total equals completed since we process until none remain
		Elapsed:       setup.summary.Duration.Nanoseconds(),
		StopReason:    setup.summary.StopReason.String(),
	}
	finalStatus.Tallies.Completed = setup.summary.Succeeded
	finalStatus.Tallies.Questions = setup.summary.Questions
	finalStatus.Tallies.Failed = setup.summary.Failed
	finalStatus.Tallies.TimedOut = setup.summary.TimedOut
	finalStatus.Tallies.Skipped = setup.summary.Skipped
	// Ignore write errors: status updates are best-effort notifications for TUI polling.
	// Loop execution continues even if status file cannot be written.
	_ = setup.statusWriter.Write(finalStatus)

	// Count remaining beads for summary
	remainingBeads := 0
	if setup.summary.StopReason == StopNormal {
		// Query one more time to see if any remain
		remaining, err := setup.fetchChildren()
		if err == nil {
			remainingBeads = len(remaining)
		}
	}

	// Print final summary
	_, _ = fmt.Fprintf(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))
}

// runEpicOrchestrator orchestrates epic leaf tasks sequentially, then runs opus verification.
// This is invoked when --epic flag is set (and --bead is not set).
// It queries 'bd ready --parent <epic-id>' after each completion to get fresh state
// and handle newly ready leaves, continuing until all leaves are done.
func runEpicOrchestrator(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	ctx, setup, cleanup, err := setupEpicOrchestrator(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loopStart := time.Now()

	// Main loop: query for ready leaves, process first one, repeat until none remain
	iteration := 0
	for {
		shouldContinue, err := processEpicIteration(ctx, cfg, setup, loopStart, iteration)
		if err != nil {
			return setup.summary, err
		}
		if !shouldContinue {
			break
		}
		iteration++
	}

	// If all children completed successfully, run opus verification
	runOpusVerification(ctx, cfg, setup)

	// Write final status and summary
	writeFinalEpicStatus(cfg, setup, loopStart)

	return setup.summary, nil
}

// sequentialLoopSetup holds setup state for sequential loop.
type sequentialLoopSetup struct {
	out                 io.Writer
	statusWriter        *StatusWriter
	summary             *RunSummary
	pickNext            func() (*beads.Bead, error)
	fetchPrompt         func(beadID string) (*PromptData, error)
	render              func(data *PromptData) (string, error)
	execute             func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn            func(beadID string, result *AgentResult) (Outcome, string)
	syncFn              func() error
	currentFormatter    *LogFormatter
	consecutiveLimit    int
	consecutiveFailures int
	lastFailedBeadID    string
	skippedBeads        map[string]bool
}

// setupSequentialLoop initializes the sequential loop setup.
func setupSequentialLoop(ctx context.Context, cfg LoopConfig) (context.Context, *sequentialLoopSetup, func(), error) {
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

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
				outBytes, err := showCmd.Output()
				if err != nil {
					return nil, fmt.Errorf("bd show %s: %w", cfg.TargetBead, err)
				}
				// bd show returns an array with one entry containing id, title, description
				var showEntries []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				}
				if err := json.Unmarshal(outBytes, &showEntries); err != nil {
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

	// Track formatter for summary output
	var currentFormatter *LogFormatter

	execute := cfg.Execute
	if execute == nil {
		execute = func(ctx context.Context, prompt string) (*AgentResult, error) {
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			// Wrap stdout with log formatter if not verbose
			if !cfg.Verbose {
				currentFormatter = NewLogFormatter(out, false)
				opts = append(opts, WithStdoutWriter(currentFormatter))
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

	setup := &sequentialLoopSetup{
		out:                 out,
		statusWriter:        statusWriter,
		summary:             summary,
		pickNext:            pickNext,
		fetchPrompt:         fetchPrompt,
		render:              render,
		execute:             execute,
		assessFn:            assessFn,
		syncFn:              syncFn,
		currentFormatter:    currentFormatter,
		consecutiveLimit:    consecutiveLimit,
		consecutiveFailures: consecutiveFailures,
		lastFailedBeadID:    lastFailedBeadID,
		skippedBeads:        skippedBeads,
	}

	cleanup := func() {
		cancelWall()
		_ = statusWriter.Clear()
	}

	return ctx, setup, cleanup, nil
}

// writeStatus writes status for sequential loop.
func writeStatus(setup *sequentialLoopSetup, state string, iteration, maxIterations int, loopStart time.Time, currentBead *BeadInfo, stopReason string) {
	status := Status{
		State:         state,
		Iteration:     iteration,
		MaxIterations: maxIterations,
		CurrentBead:   currentBead,
		Elapsed:       time.Since(loopStart).Nanoseconds(),
		StopReason:    stopReason,
	}
	status.Tallies.Completed = setup.summary.Succeeded
	status.Tallies.Questions = setup.summary.Questions
	status.Tallies.Failed = setup.summary.Failed
	status.Tallies.TimedOut = setup.summary.TimedOut
	status.Tallies.Skipped = setup.summary.Skipped
	// Ignore write errors: status updates are best-effort notifications for TUI polling.
	// Loop execution continues even if status file cannot be written.
	_ = setup.statusWriter.Write(status)
}

// handleSameBeadRetry handles same-bead retry detection logic.
func handleSameBeadRetry(setup *sequentialLoopSetup, bead *beads.Bead, cfg LoopConfig, loopStart time.Time, i int) (*beads.Bead, bool, error) {
	if setup.lastFailedBeadID == "" || bead.ID != setup.lastFailedBeadID {
		return bead, true, nil
	}

	setup.skippedBeads[bead.ID] = true
	setup.summary.Skipped++
	setup.lastFailedBeadID = "" // reset so we don't skip indefinitely

	// Check if we should stop: if we've skipped beads and the picker
	// keeps returning skipped beads, we're in an infinite loop.
	// Try one more pick; if that's also skipped, stop.
	retryBead, retryErr := setup.pickNext()
	if retryErr != nil {
		setup.summary.Duration = time.Since(loopStart)
		return nil, false, fmt.Errorf("iteration %d: picking bead (retry): %w", i+1, retryErr)
	}
	if retryBead == nil {
		setup.summary.StopReason = StopNormal
		return nil, false, nil
	}
	if setup.skippedBeads[retryBead.ID] {
		setup.summary.Skipped++
		setup.summary.StopReason = StopAllBeadsSkipped
		// Write final status before breaking
		writeStatus(setup, "completed", setup.summary.Iterations, cfg.MaxIterations, loopStart, nil, setup.summary.StopReason.String())
		return nil, false, nil
	}
	return retryBead, true, nil
}

// executeAgentForBead fetches prompt, renders it, executes agent, and prints summary.
func executeAgentForBead(ctx context.Context, cfg LoopConfig, setup *sequentialLoopSetup, bead *beads.Bead, loopStart time.Time, i int) (string, *AgentResult, error) {
	// Fetch full bead data and render prompt.
	promptData, err := setup.fetchPrompt(bead.ID)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: fetching prompt data for %s: %w",
			i+1, bead.ID, err)
	}

	prompt, err := setup.render(promptData)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: rendering prompt for %s: %w",
			i+1, bead.ID, err)
	}

	// Get commit hash before agent execution for landing check.
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = cfg.WorkDir
	commitHashBefore := ""
	if outBytes, err := cmd.Output(); err == nil {
		commitHashBefore = strings.TrimSpace(string(outBytes))
	}

	// Set current bead for progress display
	if setup.currentFormatter != nil {
		setup.currentFormatter.SetCurrentBead(bead.ID, bead.Title)
	}

	// Execute agent.
	result, err := setup.execute(ctx, prompt)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: running agent for %s: %w",
			i+1, bead.ID, err)
	}

	// Print summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		// Clear progress line before printing summary
		_, _ = fmt.Fprintf(setup.out, "\n")
		if summaryStr := setup.currentFormatter.Summary(); summaryStr != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summaryStr)
		}
		setup.currentFormatter = nil // Reset for next iteration
	}

	return commitHashBefore, result, nil
}

// handleLandingAndLogging checks landing status and logs iteration results.
func handleLandingAndLogging(cfg LoopConfig, setup *sequentialLoopSetup, bead *beads.Bead, commitHashBefore string, outcome Outcome, outcomeSummary string, result *AgentResult, i int) Outcome {
	// Check landing status.
	landingStatus, landingErr := CheckLanding(cfg.WorkDir, bead.ID, commitHashBefore)
	if landingErr == nil {
		landingMsg := FormatLandingStatus(landingStatus)
		if landingMsg != "landed successfully" {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
			// If strict landing is enabled and landing is incomplete, treat as failure
			if cfg.StrictLanding && outcome == OutcomeSuccess {
				// Override success if landing is incomplete
				if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
					outcome = OutcomeFailure
					outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
				}
			}
		} else {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
		}
	}

	// Print structured per-iteration log line.
	_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))
	// Print bead summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		if summary := setup.currentFormatter.BeadSummary(); summary != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summary)
		}
	}

	return outcome
}

// printVerboseOutput prints verbose agent output (stdout/stderr excerpts).
func printVerboseOutput(out io.Writer, result *AgentResult) {
	if result.Stdout != "" {
		lines := strings.Split(result.Stdout, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			_, _ = fmt.Fprintf(out, "  stdout (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				_, _ = fmt.Fprintf(out, "    %s\n", line)
			}
		} else {
			_, _ = fmt.Fprintf(out, "  stdout:\n")
			for _, line := range lines {
				if line != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", line)
				}
			}
		}
	}
	if result.Stderr != "" {
		lines := strings.Split(result.Stderr, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			_, _ = fmt.Fprintf(out, "  stderr (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				_, _ = fmt.Fprintf(out, "    %s\n", line)
			}
		} else {
			_, _ = fmt.Fprintf(out, "  stderr:\n")
			for _, line := range lines {
				if line != "" {
					_, _ = fmt.Fprintf(out, "    %s\n", line)
				}
			}
		}
	}
}

// updateOutcomeCounters updates counters based on outcome.
func updateOutcomeCounters(setup *sequentialLoopSetup, bead *beads.Bead, outcome Outcome) {
	setup.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		setup.summary.Succeeded++
		setup.consecutiveFailures = 0
		setup.lastFailedBeadID = ""
	case OutcomeQuestion:
		setup.summary.Questions++
		setup.consecutiveFailures = 0
		setup.lastFailedBeadID = ""
	case OutcomeFailure:
		setup.summary.Failed++
		setup.consecutiveFailures++
		setup.lastFailedBeadID = bead.ID
	case OutcomeTimeout:
		setup.summary.TimedOut++
		setup.consecutiveFailures++
		setup.lastFailedBeadID = bead.ID
	}
}

// processSequentialIteration processes a single sequential iteration.
func processSequentialIteration(ctx context.Context, cfg LoopConfig, setup *sequentialLoopSetup, loopStart time.Time, i int) (bool, error) {
	// Guard: context cancellation / wall-clock timeout.
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			setup.summary.StopReason = StopWallClock
		} else {
			setup.summary.StopReason = StopContextCancelled
		}
		// Write final status before breaking
		writeStatus(setup, "completed", setup.summary.Iterations, cfg.MaxIterations, loopStart, nil, setup.summary.StopReason.String())
		return false, nil
	}

	// 1. Pick next bead.
	bead, err := setup.pickNext()
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: picking bead: %w", i+1, err)
	}
	if bead == nil {
		setup.summary.StopReason = StopNormal
		// Write final status before breaking
		writeStatus(setup, "completed", setup.summary.Iterations, cfg.MaxIterations, loopStart, nil, setup.summary.StopReason.String())
		return false, nil
	}

	// Write status: starting iteration with current bead.
	currentBead := &BeadInfo{ID: bead.ID, Title: bead.Title}
	writeStatus(setup, "running", i+1, cfg.MaxIterations, loopStart, currentBead, "")

	// Guard: same-bead retry detection.
	bead, shouldContinue, err := handleSameBeadRetry(setup, bead, cfg, loopStart, i)
	if err != nil {
		return false, err
	}
	if !shouldContinue {
		return false, nil
	}

	// Dry-run: print what would be done without executing.
	if cfg.DryRun {
		_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
		setup.summary.Iterations++
		// Write final status for dry-run
		writeStatus(setup, "completed", setup.summary.Iterations, cfg.MaxIterations, loopStart, nil, "dry-run")
		return false, nil
	}

	// 2. Execute agent for bead.
	commitHashBefore, result, err := executeAgentForBead(ctx, cfg, setup, bead, loopStart, i)
	if err != nil {
		return false, err
	}

	// 3. Assess outcome.
	outcome, outcomeSummary := setup.assessFn(bead.ID, result)

	// 4. Handle landing and logging.
	outcome = handleLandingAndLogging(cfg, setup, bead, commitHashBefore, outcome, outcomeSummary, result, i)

	// Verbose mode: print agent stdout/stderr excerpts.
	if cfg.Verbose {
		printVerboseOutput(setup.out, result)
	}

	// 5. Update counters.
	updateOutcomeCounters(setup, bead, outcome)

	// Write status update after iteration completes.
	writeStatus(setup, "running", i+1, cfg.MaxIterations, loopStart, nil, "")

	// Guard: consecutive failure limit.
	if setup.consecutiveFailures >= setup.consecutiveLimit {
		setup.summary.StopReason = StopConsecutiveFails
		// Write final status before breaking
		writeStatus(setup, "completed", i+1, cfg.MaxIterations, loopStart, nil, setup.summary.StopReason.String())
		return false, nil
	}

	// 6. Sync beads state.
	if err := setup.syncFn(); err != nil {
		if cfg.Verbose {
			_, _ = fmt.Fprintf(setup.out, "  bd sync warning: %v\n", err)
		}
	}

	return true, nil
}

// writeFinalSequentialStatus writes the final status and summary for sequential loop.
func writeFinalSequentialStatus(cfg LoopConfig, setup *sequentialLoopSetup, loopStart time.Time) {
	// If we exhausted all iterations without an earlier stop reason, set it.
	if setup.summary.StopReason == StopNormal && setup.summary.Iterations >= cfg.MaxIterations {
		setup.summary.StopReason = StopMaxIterations
	}

	// Calculate total duration.
	setup.summary.Duration = time.Since(loopStart)

	// Write final status.
	writeStatus(setup, "completed", setup.summary.Iterations, cfg.MaxIterations, loopStart, nil, setup.summary.StopReason.String())

	// Count remaining beads.
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary (always printed, even on early termination).
	_, _ = fmt.Fprintf(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))
}

// runSequential executes the sequential loop (original implementation).
func runSequential(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Epic mode: when Epic is set and TargetBead is empty, orchestrate children sequentially
	if cfg.Epic != "" && cfg.TargetBead == "" {
		return runEpicOrchestrator(ctx, cfg)
	}

	ctx, setup, cleanup, err := setupSequentialLoop(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loopStart := time.Now()

	for i := 0; i < cfg.MaxIterations; i++ {
		shouldContinue, err := processSequentialIteration(ctx, cfg, setup, loopStart, i)
		if err != nil {
			return setup.summary, err
		}
		if !shouldContinue {
			break
		}
	}

	// Write final status and summary
	writeFinalSequentialStatus(cfg, setup, loopStart)

	return setup.summary, nil
}

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
	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

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
	summary := &RunSummary{}
	consecutiveFailures := int32(0)
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)
	iterations := int32(0)
	stopReason := StopNormal
	shouldStop := int32(0) // atomic flag for early termination

	setup := &concurrentLoopSetup{
		out:                 out,
		summary:             summary,
		wtMgr:               wtMgr,
		pickNext:            pickNext,
		fetchPrompt:         fetchPrompt,
		render:              render,
		assessFn:            assessFn,
		syncFn:              syncFn,
		consecutiveLimit:    consecutiveLimit,
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
		_, _ = fmt.Fprintf(setup.out, "[worker %d] failed to fetch prompt for %s: %v\n", workerID, bead.ID, err)
		setup.mu.Unlock()
		return nil, err
	}

	// Render prompt
	prompt, err := setup.render(promptData)
	if err != nil {
		setup.mu.Lock()
		_, _ = fmt.Fprintf(setup.out, "[worker %d] failed to render prompt for %s: %v\n", workerID, bead.ID, err)
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
		_, _ = fmt.Fprintf(setup.out, "[worker %d] failed to run agent for %s: %v\n", workerID, bead.ID, err)
		setup.mu.Unlock()
		return nil, err
	}

	return result, nil
}

// logWorkerIteration logs iteration results and verbose output.
func logWorkerIteration(setup *concurrentLoopSetup, cfg LoopConfig, iterNum int, bead *beads.Bead, outcome Outcome, result *AgentResult, outcomeSummary string) {
	// Print structured per-iteration log line
	_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))
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
				_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(iterNum, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
				setup.summary.Iterations++
				atomic.StoreInt32(&setup.shouldStop, 1)
				setup.mu.Unlock()
				return
			}

			// Create worktree for this bead
			worktreePath, branchName, err := setup.wtMgr.CreateWorktree(bead.ID)
			if err != nil {
				setup.mu.Lock()
				_, _ = fmt.Fprintf(setup.out, "[worker %d] failed to create worktree for %s: %v\n", workerID, bead.ID, err)
				setup.mu.Unlock()
				continue
			}
			defer func() {
				if err := setup.wtMgr.RemoveWorktree(worktreePath, branchName); err != nil {
					setup.mu.Lock()
					_, _ = fmt.Fprintf(setup.out, "[worker %d] warning: failed to remove worktree %s: %v\n", workerID, worktreePath, err)
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
					_, _ = fmt.Fprintf(setup.out, "  bd sync warning: %v\n", err)
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
	_, _ = fmt.Fprintf(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))

	return setup.summary, nil
}
