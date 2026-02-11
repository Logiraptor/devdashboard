package ralph

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIModel_Init(t *testing.T) {
	cfg := LoopConfig{
		WorkDir:       "/tmp/test",
		MaxIterations: 5,
	}
	model := NewTUIModel(cfg)

	if model == nil {
		t.Fatal("NewTUIModel returned nil")
	}
	if model.traceView == nil {
		t.Error("traceView should be initialized")
	}
	if model.traceEmitter == nil {
		t.Error("traceEmitter should be initialized")
	}
}

func TestTUIModel_HandleLoopStarted(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := LoopStartedMsg{TraceID: "abc123", Epic: "test-epic"}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if !m.loopStarted {
		t.Error("loopStarted should be true")
	}
	if m.status == "" {
		t.Error("status should be set")
	}
}

func TestTUIModel_HandleIterationEnd(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	// Process a successful iteration
	msg := IterationEndMsg{
		BeadID:   "bead-1",
		Outcome:  OutcomeSuccess,
		Duration: 30 * time.Second,
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if m.summary.Succeeded != 1 {
		t.Errorf("Succeeded should be 1, got %d", m.summary.Succeeded)
	}
	if m.summary.Iterations != 1 {
		t.Errorf("Iterations should be 1, got %d", m.summary.Iterations)
	}
}

func TestTUIModel_HandleIterationEnd_Failure(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := IterationEndMsg{
		BeadID:   "bead-1",
		Outcome:  OutcomeFailure,
		Duration: 15 * time.Second,
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if m.summary.Failed != 1 {
		t.Errorf("Failed should be 1, got %d", m.summary.Failed)
	}
	if m.summary.Succeeded != 0 {
		t.Errorf("Succeeded should be 0, got %d", m.summary.Succeeded)
	}
}

func TestTUIModel_HandleIterationEnd_AllOutcomes(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	outcomes := []Outcome{OutcomeSuccess, OutcomeFailure, OutcomeTimeout, OutcomeQuestion}
	for _, outcome := range outcomes {
		msg := IterationEndMsg{
			BeadID:   "bead-1",
			Outcome:  outcome,
			Duration: 10 * time.Second,
		}
		newModel, _ := model.Update(msg)
		model = newModel.(*TUIModel)
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

func TestTUIModel_WindowResize(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if m.width != 120 {
		t.Errorf("width should be 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("height should be 40, got %d", m.height)
	}
}

func TestTUIModel_HandleIterationStart(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := IterationStartMsg{
		BeadID:    "bead-123",
		BeadTitle: "Test Bead",
		IterNum:   1,
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if m.status == "" {
		t.Error("status should be set after iteration start")
	}
}

func TestTUIModel_HandleLoopEnd(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	summary := &RunSummary{
		Iterations: 5,
		Succeeded:  3,
		Failed:     2,
		StopReason: StopNormal,
		Duration:   10 * time.Minute,
	}
	msg := LoopEndMsg{
		Summary:    summary,
		StopReason: StopNormal,
	}
	newModel, _ := model.Update(msg)

	m := newModel.(*TUIModel)
	if !m.loopDone {
		t.Error("loopDone should be true")
	}
	if m.summary.Iterations != 5 {
		t.Errorf("summary.Iterations should be 5, got %d", m.summary.Iterations)
	}
}

func TestTUIModel_HandleLoopError(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := LoopErrorMsg{Err: &testError{message: "test error"}}
	newModel, cmd := model.Update(msg)

	m := newModel.(*TUIModel)
	if m.err == nil {
		t.Error("err should be set")
	}
	if cmd == nil {
		t.Error("should return quit command")
	}
}

func TestTUIModel_HandleKeyQuit(t *testing.T) {
	cfg := LoopConfig{WorkDir: "/tmp/test"}
	model := NewTUIModel(cfg)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	newModel, cmd := model.Update(msg)

	if cmd == nil {
		t.Error("should return quit command")
	}
	_ = newModel // Model may be unchanged
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
