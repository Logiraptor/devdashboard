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
type StubRunner struct{}

// Run implements Runner. Emits fake progress events as tea.Msg.
// Phase 6 will consume these for live display.
func (s *StubRunner) Run(ctx context.Context, projectDir, planPath, designPath string) tea.Cmd {
	return func() tea.Msg {
		return progress.Event{
			Message:   "Agent run started (stub) — " + filepath.Base(projectDir),
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
		}
	}
}
