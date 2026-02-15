package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// MarshalJSON implements json.Marshaler.
func (r StopReason) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *StopReason) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "normal":
		*r = StopNormal
	case "max-iterations":
		*r = StopMaxIterations
	case "consecutive-failures":
		*r = StopConsecutiveFails
	case "wall-clock-timeout":
		*r = StopWallClock
	case "context-cancelled":
		*r = StopContextCancelled
	case "all-beads-skipped":
		*r = StopAllBeadsSkipped
	default:
		return fmt.Errorf("unknown StopReason: %s", s)
	}
	return nil
}

// DefaultConsecutiveFailureLimit is the number of consecutive failures before
// the loop stops. Set to 3 to allow for transient failures (e.g., network issues,
// flaky tests) while still catching persistent problems quickly.
const DefaultConsecutiveFailureLimit = 3

// DefaultWallClockTimeout is the maximum total duration for a ralph session.
// Set to 2 hours to allow for substantial work while preventing runaway sessions.
// Individual agent timeouts are controlled separately via LoopConfig.AgentTimeout.
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

	// Progress callbacks (all optional) — called during execution for TUI integration.
	// If nil, no-op.
	OnBatchStart   func(batch []beads.Bead, batchNum int)
	OnBeadStart    func(bead beads.Bead)
	OnBeadEnd      func(bead beads.Bead, outcome Outcome, duration time.Duration)
	OnBeadComplete func(result *BeadResult) // Called with full result including agent details
	OnBatchEnd     func(batchNum int, results []BeadResult)
}

// BeadResult holds the result of executing a single bead.
type BeadResult struct {
	Bead     beads.Bead
	Outcome  Outcome
	Duration time.Duration

	// Agent execution details (for debugging failures)
	ChatID       string // Agent chat session ID
	ErrorMessage string // Error message from the agent
	ExitCode     int    // Agent process exit code
	Stderr       string // Stderr output from the agent
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

// FormatDuration formats a duration in a human-readable way (e.g., "2m34s", "1h12m").
func FormatDuration(d time.Duration) string {
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
	var status strings.Builder
	switch outcome {
	case OutcomeSuccess:
		status.WriteString("success")
	case OutcomeQuestion:
		// Extract question bead IDs from outcomeSummary: "bead X has N question(s) needing human input: id1, id2"
		status.WriteString("question")
		if strings.Contains(outcomeSummary, ": ") {
			parts := strings.Split(outcomeSummary, ": ")
			if len(parts) > 1 {
				questionIDs := strings.TrimSpace(parts[1])
				status.Reset()
				fmt.Fprintf(&status, "question: %s", questionIDs)
			}
		}
	case OutcomeFailure:
		status.WriteString("failed")
		// Extract exit code from: "bead X still open after agent run (exit code N, duration ...)"
		// or: "failed to query bead X: ... (agent exit code N)"
		if strings.Contains(outcomeSummary, "exit code") {
			// Find "exit code" and extract the number after it
			idx := strings.Index(outcomeSummary, "exit code")
			if idx >= 0 {
				afterCode := outcomeSummary[idx+len("exit code"):]
				afterCode = strings.TrimSpace(afterCode)
				// Extract first number
				var exitCode strings.Builder
				for _, r := range afterCode {
					if r >= '0' && r <= '9' {
						exitCode.WriteRune(r)
					} else if exitCode.Len() > 0 {
						break
					}
				}
				if exitCode.Len() > 0 {
					status.Reset()
					fmt.Fprintf(&status, "failed: exit code %s", exitCode.String())
				}
			}
		}
	case OutcomeTimeout:
		status.WriteString("timeout")
		// Extract exit code from: "agent timed out after ... (exit code N)"
		if strings.Contains(outcomeSummary, "exit code") {
			idx := strings.Index(outcomeSummary, "exit code")
			if idx >= 0 {
				afterCode := outcomeSummary[idx+len("exit code"):]
				afterCode = strings.TrimSpace(afterCode)
				// Extract first number
				var exitCode strings.Builder
				for _, r := range afterCode {
					if r >= '0' && r <= '9' {
						exitCode.WriteRune(r)
					} else if exitCode.Len() > 0 {
						break
					}
				}
				if exitCode.Len() > 0 {
					status.Reset()
					fmt.Fprintf(&status, "timeout: exit code %s", exitCode.String())
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
					status.Reset()
					fmt.Fprintf(&status, "timeout (%s)", timeoutDur)
				}
			}
		}
	}

	var result strings.Builder
	fmt.Fprintf(&result, "[%d/%d] %s \"%s\" → %s (%s)",
		iter, maxIter, beadID, title, status.String(), FormatDuration(duration))
	return result.String()
}

// formatSummary formats the end-of-loop summary.
func formatSummary(summary *RunSummary, remainingBeads int) string {
	var b strings.Builder
	b.WriteString("Ralph loop complete:\n")

	if summary.Succeeded > 0 {
		fmt.Fprintf(&b, "  ✓ %d beads completed\n", summary.Succeeded)
	}
	if summary.Questions > 0 {
		fmt.Fprintf(&b, "  ? %d questions created (needs human)\n", summary.Questions)
	}
	if summary.Failed > 0 {
		fmt.Fprintf(&b, "  ✗ %d failure(s)\n", summary.Failed)
	}
	if summary.TimedOut > 0 {
		fmt.Fprintf(&b, "  ⏱ %d timeout(s)\n", summary.TimedOut)
	}
	if summary.Skipped > 0 {
		fmt.Fprintf(&b, "  ⊘ %d skipped\n", summary.Skipped)
	}
	if remainingBeads > 0 {
		fmt.Fprintf(&b, "  ○ %d beads remaining (blocked)\n", remainingBeads)
	}

	fmt.Fprintf(&b, "  Duration: %s", FormatDuration(summary.Duration))

	return b.String()
}

// countRemainingBeads counts the number of ready beads remaining.
func countRemainingBeads(cfg LoopConfig) int {
	if cfg.PickNext == nil {
		beads, err := ReadyBeads(cfg.WorkDir, cfg.Epic)
		if err != nil {
			return 0
		}
		return len(beads)
	}

	// With a custom picker, we can't easily count
	// Try to pick one to see if any remain
	bead, err := cfg.PickNext()
	if err != nil || bead == nil {
		return 0
	}
	return 1 // At least one remains
}
