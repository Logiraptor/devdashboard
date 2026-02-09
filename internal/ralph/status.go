package ralph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Status represents the current state of a ralph loop execution.
// This is written to a JSON file for devdeploy TUI to poll.
type Status struct {
	// State indicates whether ralph is running or has completed.
	State string `json:"state"` // "running" or "completed"

	// Current iteration number (1-indexed).
	Iteration int `json:"iteration"`

	// MaxIterations is the configured maximum iterations.
	MaxIterations int `json:"max_iterations"`

	// CurrentBead is the bead currently being worked on (nil if none).
	CurrentBead *BeadInfo `json:"current_bead,omitempty"`

	// Elapsed is the total elapsed time since the loop started (in nanoseconds).
	Elapsed int64 `json:"elapsed_ns"`

	// Tallies are running counts of outcomes.
	Tallies struct {
		Completed int `json:"completed"`
		Questions int `json:"questions"`
		Failed    int `json:"failed"`
		TimedOut  int `json:"timed_out"`
		Skipped   int `json:"skipped"`
	} `json:"tallies"`

	// StopReason indicates why the loop stopped (only set when state="completed").
	StopReason string `json:"stop_reason,omitempty"`
}

// BeadInfo represents minimal information about a bead.
type BeadInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// StatusWriter manages writing status updates to a file.
type StatusWriter struct {
	path string
}

// NewStatusWriter creates a new StatusWriter that writes to the given path.
func NewStatusWriter(workdir string) *StatusWriter {
	return &StatusWriter{
		path: filepath.Join(workdir, ".ralph-status.json"),
	}
}

// Write updates the status file with the current state.
func (w *StatusWriter) Write(status Status) error {
	// Marshal to JSON with indentation for readability.
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}

	// Write atomically: write to temp file, then rename.
	tmpPath := w.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, w.path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// Clear removes the status file (called when ralph completes).
func (w *StatusWriter) Clear() error {
	if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove status file: %w", err)
	}
	return nil
}
