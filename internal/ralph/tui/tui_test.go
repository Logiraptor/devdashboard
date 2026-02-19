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
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
	}
	model := NewModel(core)

	if model == nil {
		t.Fatal("NewModel returned nil")
	}
	if model.maxIter != 10 {
		t.Errorf("maxIter should be 10, got %d", model.maxIter)
	}
}

func TestModel_HandleLoopStarted(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
	model := NewModel(core)

	msg := loopStartedMsg{RootBead: "test-bead"}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if !m.loopStarted {
		t.Error("loopStarted should be true")
	}
	if m.beadID != "test-bead" {
		t.Errorf("beadID should be 'test-bead', got %q", m.beadID)
	}
}

func TestModel_HandleBeadStart(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
	model := NewModel(core)

	msg := beadStartMsg{
		Bead: beads.Bead{ID: "bead-123", Title: "Test Bead"},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.beadID != "bead-123" {
		t.Errorf("beadID should be 'bead-123', got %q", m.beadID)
	}
	if m.beadTitle != "Test Bead" {
		t.Errorf("beadTitle should be 'Test Bead', got %q", m.beadTitle)
	}
}

func TestModel_HandleIterationStart(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead", MaxIterations: 10}
	model := NewModel(core)

	msg := iterationStartMsg{Iteration: 2}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.iteration != 3 { // 0-indexed to 1-indexed
		t.Errorf("iteration should be 3, got %d", m.iteration)
	}
}

func TestModel_HandleToolEvent(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
	model := NewModel(core)

	// Tool start
	startMsg := toolEventMsg{
		Event:   ralph.ToolEvent{Name: "Read"},
		Started: true,
	}
	newModel, _ := model.Update(startMsg)
	m := newModel.(*Model)
	if m.currentTool != "Read" {
		t.Errorf("currentTool should be 'Read', got %q", m.currentTool)
	}

	// Tool end
	endMsg := toolEventMsg{
		Event:   ralph.ToolEvent{Name: "Read"},
		Started: false,
	}
	newModel, _ = m.Update(endMsg)
	m = newModel.(*Model)
	if m.currentTool != "" {
		t.Errorf("currentTool should be empty after tool end, got %q", m.currentTool)
	}
}

func TestModel_HandleBeadComplete(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
	model := NewModel(core)

	msg := beadCompleteMsg{
		Result: ralph.BeadResult{
			Bead:     beads.Bead{ID: "bead-1", Title: "Test Bead"},
			Outcome:  ralph.OutcomeSuccess,
			Duration: 30 * time.Second,
		},
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if m.outcome != ralph.OutcomeSuccess {
		t.Errorf("outcome should be success, got %v", m.outcome)
	}
}

func TestModel_HandleLoopEnd(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
	model := NewModel(core)

	result := &ralph.CoreResult{
		Outcome:    ralph.OutcomeSuccess,
		Iterations: 3,
		Duration:   10 * time.Minute,
	}
	msg := loopEndMsg{Result: result}
	newModel, _ := model.Update(msg)

	m := newModel.(*Model)
	if !m.loopDone {
		t.Error("loopDone should be true")
	}
	if m.outcome != ralph.OutcomeSuccess {
		t.Errorf("outcome should be success, got %v", m.outcome)
	}
	if m.iteration != 3 {
		t.Errorf("iteration should be 3, got %d", m.iteration)
	}
	if m.duration != 10*time.Minute {
		t.Errorf("duration should be 10m, got %v", m.duration)
	}
}

func TestModel_HandleLoopError(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
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

func TestModel_WindowResize(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
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

func TestModel_HandleKeyQuit(t *testing.T) {
	core := &ralph.Core{WorkDir: "/tmp/test", RootBead: "test-bead"}
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
