package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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
	Project       string
	Epic          string
	Labels        []string
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
			Project: cfg.Project,
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
func Run(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
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
		picker := &BeadPicker{
			WorkDir: cfg.WorkDir,
			Project: cfg.Project,
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

	summary := &RunSummary{}
	consecutiveFailures := 0
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)

	for i := 0; i < cfg.MaxIterations; i++ {
		// Guard: context cancellation / wall-clock timeout.
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				summary.StopReason = StopWallClock
			} else {
				summary.StopReason = StopContextCancelled
			}
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
			break
		}

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
				break
			}
			bead = retryBead
		}

		// Dry-run: print what would be done without executing.
		if cfg.DryRun {
			fmt.Fprintf(out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
			summary.Iterations++
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

		// 3. Execute agent.
		result, err := execute(ctx, prompt)
		if err != nil {
			summary.Duration = time.Since(loopStart)
			return summary, fmt.Errorf("iteration %d: running agent for %s: %w",
				i+1, bead.ID, err)
		}

		// 4. Assess outcome.
		outcome, outcomeSummary := assessFn(bead.ID, result)

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

		// Guard: consecutive failure limit.
		if consecutiveFailures >= consecutiveLimit {
			summary.StopReason = StopConsecutiveFails
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

	// Count remaining beads.
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary (always printed, even on early termination).
	fmt.Fprintf(out, "\n%s\n", formatSummary(summary, remainingBeads))

	return summary, nil
}
