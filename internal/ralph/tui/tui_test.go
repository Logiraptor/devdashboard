package tui

import (
	"testing"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/ralph"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_Init(t *testing.T) {
	core := &ralph.Core{
		WorkDir:  "/tmp/test",
		RootBead: "test-epic",
	}
	model := NewModel(core)

	if model == nil {
		t.Fatal("NewModel returned nil")
	}
	if model.multiAgentView == nil {
		t.Error("multiAgentView should be initialized")
	}
	if model.toolToBeadMap == nil {
		t.Error("toolToBeadMap should be initialized")
	}
}

func TestModel_HandleLoopStarted(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := loopStartedMsg{RootBead: "test-epic"}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if !m.loopStarted {
		t.Error("loopStarted should be true")
	}
	if m.status == "" {
		t.Error("status should be set")
	}
}

func TestModel_HandleBeadComplete(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	// Process a successful bead
	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-1", Title: "Test Bead"},
			Outcome:  ralph.OutcomeSuccess,
			Duration: 30 * time.Second,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.summary.Succeeded != 1 {
		t.Errorf("Succeeded should be 1, got %d", m.summary.Succeeded)
	}
	if m.summary.Iterations != 1 {
		t.Errorf("Iterations should be 1, got %d", m.summary.Iterations)
	}
}

func TestModel_HandleBeadComplete_Failure(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-1", Title: "Failing Bead"},
			Outcome:  ralph.OutcomeFailure,
			Duration: 15 * time.Second,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.summary.Failed != 1 {
		t.Errorf("Failed should be 1, got %d", m.summary.Failed)
	}
	if m.summary.Succeeded != 0 {
		t.Errorf("Succeeded should be 0, got %d", m.summary.Succeeded)
	}
}

func TestModel_HandleBeadComplete_FailureTracking(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	// Send a failure with ChatID and error info
	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:         beads.Bead{ID: "bead-fail", Title: "Failed Bead"},
			Outcome:      ralph.OutcomeFailure,
			Duration:     20 * time.Second,
			ChatID:       "chat-123abc",
			ErrorMessage: "Agent crashed",
			ExitCode:     1,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.lastFailure == nil {
		t.Fatal("lastFailure should be set")
	}
	if m.lastFailure.ChatID != "chat-123abc" {
		t.Errorf("ChatID = %q, want %q", m.lastFailure.ChatID, "chat-123abc")
	}
	if m.lastFailure.ErrorMessage != "Agent crashed" {
		t.Errorf("ErrorMessage = %q, want %q", m.lastFailure.ErrorMessage, "Agent crashed")
	}
	if m.lastFailure.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", m.lastFailure.ExitCode)
	}
}

func TestModel_HandleBeadComplete_StderrFallback(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	// First start the bead so it's tracked in the view
	startMsg := beadStartMsg{
		Bead: beads.Bead{ID: "bead-stderr", Title: "Stderr Bead"},
	}
	model.Update(startMsg)

	// Send a failure with Stderr but no ErrorMessage
	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-stderr", Title: "Stderr Bead"},
			Outcome:  ralph.OutcomeFailure,
			Duration: 15 * time.Second,
			ExitCode: 1,
			Stderr:   "panic: runtime error: invalid memory address\ngoroutine 1 [running]:\nmain.main()\n\t/app/main.go:42 +0x123",
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.lastFailure == nil {
		t.Fatal("lastFailure should be set")
	}
	if m.lastFailure.Stderr == "" {
		t.Error("Stderr should be preserved")
	}
}

func TestModel_HandleBeadComplete_NoErrorInfo(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	// First start the bead so it's tracked in the view
	startMsg := beadStartMsg{
		Bead: beads.Bead{ID: "bead-empty", Title: "Empty Error Bead"},
	}
	model.Update(startMsg)

	// Send a failure with no error info at all
	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-empty", Title: "Empty Error Bead"},
			Outcome:  ralph.OutcomeFailure,
			Duration: 10 * time.Second,
			ExitCode: 1,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.lastFailure == nil {
		t.Fatal("lastFailure should be set")
	}
	// The new TUI displays failures in agent blocks, not in a separate section
	// Just verify the failure was tracked
	if m.lastFailure.ExitCode != 1 {
		t.Errorf("ExitCode should be 1, got %d", m.lastFailure.ExitCode)
	}
}

func TestModel_HandleBeadComplete_TimeoutTracking(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	// Timeouts should also be tracked
	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-timeout", Title: "Timeout Bead"},
			Outcome:  ralph.OutcomeTimeout,
			Duration: 10 * time.Minute,
			ChatID:   "chat-timeout456",
			ExitCode: -1,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.lastFailure == nil {
		t.Fatal("lastFailure should be set for timeout")
	}
	if m.lastFailure.ChatID != "chat-timeout456" {
		t.Errorf("ChatID = %q, want %q", m.lastFailure.ChatID, "chat-timeout456")
	}
}

func TestModel_HandleBeadComplete_AllOutcomes(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	outcomes := []ralph.Outcome{ralph.OutcomeSuccess, ralph.OutcomeFailure, ralph.OutcomeTimeout, ralph.OutcomeQuestion}
	for _, outcome := range outcomes {
		msg := beadCompleteMsg{
			Result: ralph.BeadResult{
				Bead:     beads.Bead{ID: "bead-1", Title: "Test"},
				Outcome:  outcome,
				Duration: 10 * time.Second,
			},
		}
		newModel, _ := model.Update(msg)
		model = newModel.(*Model)
	}

	m := model
	if m.summary.Succeeded != 1 {
		t.Errorf("Succeeded should be 1, got %d", m.summary.Succeeded)
	}
	if m.summary.Failed != 1 {
		t.Errorf("Failed should be 1, got %d", m.summary.Failed)
	}
	if m.summary.TimedOut != 1 {
		t.Errorf("TimedOut should be 1, got %d", m.summary.TimedOut)
	}
	if m.summary.Questions != 1 {
		t.Errorf("Questions should be 1, got %d", m.summary.Questions)
	}
	if m.summary.Iterations != 4 {
		t.Errorf("Iterations should be 4, got %d", m.summary.Iterations)
	}
}

func TestModel_WindowResize(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.width != 120 {
		t.Errorf("width should be 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("height should be 40, got %d", m.height)
	}
}

func TestModel_HandleBeadStart(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := beadStartMsg{
		Bead: beads.Bead{ID: "bead-123", Title: "Test Bead"},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.status == "" {
		t.Error("status should be set after bead start")
	}
	// Verify agent was added to the multi-agent view
	if m.multiAgentView.TotalCount() != 1 {
		t.Errorf("multiAgentView should have 1 agent, got %d", m.multiAgentView.TotalCount())
	}
	if m.currentBead != "bead-123" {
		t.Errorf("currentBead should be 'bead-123', got %q", m.currentBead)
	}
}

func TestModel_HandleLoopEnd(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	result := &ralph.CoreResult{
		Succeeded: 3,
		Failed:    2,
		Duration:  10 * time.Minute,
	}
	msg := loopEndMsg{Result: result}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if !m.loopDone {
		t.Error("loopDone should be true")
	}
	if m.summary.Succeeded != 3 {
		t.Errorf("summary.Succeeded should be 3, got %d", m.summary.Succeeded)
	}
}

func TestModel_HandleLoopError(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := loopErrorMsg{Err: &testError{message: "test error"}}
	newModel, cmd := model.Update(msg)

	m := newModel.(*Model)
	if m.err == nil {
		t.Error("err should be set")
	}
	if cmd == nil {
		t.Error("should return quit command")
	}
}

func TestModel_HandleKeyQuit(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test"}
	model := NewModel(core)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("should return quit command")
	}
}

func TestObserver_ImplementsInterface(t *testing.T) {
	// Compile-time check that Observer implements ProgressObserver
	var _ ralph.ProgressObserver = (*Observer)(nil)
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
