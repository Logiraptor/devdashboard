package ralph

import (
	"encoding/json"
	"fmt"
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
// Individual agent timeouts are controlled separately via Core.AgentTimeout.
const DefaultWallClockTimeout = 2 * time.Hour

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
