package ralph

import (
	"fmt"
	"time"

	"devdeploy/internal/beads"
)

// StopReason indicates why the ralph loop terminated.
type StopReason int

const (
	StopNormal           StopReason = iota // Bead closed successfully.
	StopMaxIterations                      // Hit --max-iterations cap.
	StopContextCancelled                   // Context cancelled (e.g. SIGINT).
	StopQuestion                           // Agent created needs-human question.
	StopTimeout                            // Agent timed out.
)

// String returns a human-readable label for the stop reason.
func (r StopReason) String() string {
	switch r {
	case StopNormal:
		return "normal"
	case StopMaxIterations:
		return "max-iterations"
	case StopContextCancelled:
		return "context-cancelled"
	case StopQuestion:
		return "question"
	case StopTimeout:
		return "timeout"
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
	case StopQuestion:
		return 3
	case StopTimeout:
		return 4
	case StopContextCancelled:
		return 5
	default:
		return 1
	}
}

// parseStopReason converts a string to a StopReason value.
func parseStopReason(s string) (StopReason, error) {
	switch s {
	case "normal":
		return StopNormal, nil
	case "max-iterations":
		return StopMaxIterations, nil
	case "context-cancelled":
		return StopContextCancelled, nil
	case "question":
		return StopQuestion, nil
	case "timeout":
		return StopTimeout, nil
	default:
		return 0, ParseEnumError("StopReason", s)
	}
}

// MarshalJSON implements json.Marshaler.
func (r StopReason) MarshalJSON() ([]byte, error) {
	return MarshalEnumJSON(r)
}

// UnmarshalJSON implements json.Unmarshaler.
func (r *StopReason) UnmarshalJSON(data []byte) error {
	parsed, err := UnmarshalEnumJSON(data, parseStopReason)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
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
