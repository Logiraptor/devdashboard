package agent

import (
	"context"
	"path/filepath"
	"time"

	"devdeploy/internal/progress"

	tea "github.com/charmbracelet/bubbletea"
)

// Runner is the integration point for triggering agent runs.
// Implementations can be Cursor, Claude Code, or a stub.
type Runner interface {
	Run(ctx context.Context, projectDir, planPath, designPath string) tea.Cmd
}

// StubRunner emits fake progress events for Phase 6 integration testing.
// Emits a stream of events over ~2 seconds to simulate live agent output.
type StubRunner struct{}

// Run implements Runner. Emits fake progress events as tea.Msg.
// Phase 6 will consume these for live display.
// Respects ctx cancellation: when ctx is done, emits StatusAborted and stops.
func (s *StubRunner) Run(ctx context.Context, projectDir, planPath, designPath string) tea.Cmd {
	base := filepath.Base(projectDir)
	return tea.Sequence(
		emitAfter(ctx, 0, progress.Event{
			Message:   "Agent run started (stub) — " + base,
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
		}),
		emitAfter(ctx, 400*time.Millisecond, progress.Event{
			Message:   "Loading plan from " + planPath,
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
		}),
		emitAfter(ctx, 400*time.Millisecond, progress.Event{
			Message:   "Analyzing design context",
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
		}),
		emitAfter(ctx, 400*time.Millisecond, progress.Event{
			Message:   "Executing tasks...",
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
			Metadata:  map[string]string{"step": "3", "total": "5"},
		}),
		emitAfter(ctx, 400*time.Millisecond, progress.Event{
			Message:   "Agent run completed (stub)",
			Status:    progress.StatusDone,
			Timestamp: time.Now(),
		}),
	)
}

// emitAfter returns a Cmd that sleeps then emits the event.
// If ctx is cancelled during sleep, emits an aborted event instead.
func emitAfter(ctx context.Context, d time.Duration, ev progress.Event) tea.Cmd {
	return func() tea.Msg {
		if d > 0 {
			select {
			case <-ctx.Done():
				return progress.Event{
					Message:   "Aborted",
					Status:    progress.StatusAborted,
					Timestamp: time.Now(),
				}
			case <-time.After(d):
				// continue to emit
			}
		}
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
		return ev
	}
}
