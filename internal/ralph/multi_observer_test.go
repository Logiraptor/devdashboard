package ralph

import (
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// multiTestObserver tracks method calls for testing.
type multiTestObserver struct {
	onLoopStartCalls      []string
	onBeadStartCalls      []beads.Bead
	onBeadCompleteCalls   []BeadResult
	onLoopEndCalls        []*CoreResult
	onToolStartCalls      []ToolEvent
	onToolEndCalls        []ToolEvent
	onIterationStartCalls []int
	onVerifyStartCalls    []string
	onVerifyEndCalls      []VerifyResult
}

func (t *multiTestObserver) OnLoopStart(rootBead string) {
	t.onLoopStartCalls = append(t.onLoopStartCalls, rootBead)
}

func (t *multiTestObserver) OnBeadStart(bead beads.Bead) {
	t.onBeadStartCalls = append(t.onBeadStartCalls, bead)
}

func (t *multiTestObserver) OnBeadComplete(result BeadResult) {
	t.onBeadCompleteCalls = append(t.onBeadCompleteCalls, result)
}

func (t *multiTestObserver) OnLoopEnd(result *CoreResult) {
	t.onLoopEndCalls = append(t.onLoopEndCalls, result)
}

func (t *multiTestObserver) OnToolStart(event ToolEvent) {
	t.onToolStartCalls = append(t.onToolStartCalls, event)
}

func (t *multiTestObserver) OnToolEnd(event ToolEvent) {
	t.onToolEndCalls = append(t.onToolEndCalls, event)
}

func (t *multiTestObserver) OnIterationStart(iteration int) {
	t.onIterationStartCalls = append(t.onIterationStartCalls, iteration)
}

func (t *multiTestObserver) OnVerifyStart(beadID string) {
	t.onVerifyStartCalls = append(t.onVerifyStartCalls, beadID)
}

func (t *multiTestObserver) OnVerifyEnd(result VerifyResult) {
	t.onVerifyEndCalls = append(t.onVerifyEndCalls, result)
}

func TestNewMultiObserver_FiltersNilObservers(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(obs1, nil, obs2, nil)

	if len(multi.observers) != 2 {
		t.Errorf("expected 2 observers, got %d", len(multi.observers))
	}
}

func TestNewMultiObserver_AllNil(t *testing.T) {
	multi := NewMultiObserver(nil, nil, nil)

	if len(multi.observers) != 0 {
		t.Errorf("expected 0 observers, got %d", len(multi.observers))
	}
}

func TestMultiObserver_OnLoopStart(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(obs1, obs2)
	multi.OnLoopStart("test-bead")

	if len(obs1.onLoopStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopStartCalls))
	}
	if obs1.onLoopStartCalls[0] != "test-bead" {
		t.Errorf("obs1: expected 'test-bead', got %q", obs1.onLoopStartCalls[0])
	}

	if len(obs2.onLoopStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopStartCalls))
	}
	if obs2.onLoopStartCalls[0] != "test-bead" {
		t.Errorf("obs2: expected 'test-bead', got %q", obs2.onLoopStartCalls[0])
	}
}

func TestMultiObserver_OnBeadStart(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	bead := beads.Bead{ID: "bead-1", Title: "Test Bead"}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnBeadStart(bead)

	if len(obs1.onBeadStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onBeadStartCalls))
	}
	if obs1.onBeadStartCalls[0].ID != "bead-1" {
		t.Errorf("obs1: expected ID 'bead-1', got %q", obs1.onBeadStartCalls[0].ID)
	}

	if len(obs2.onBeadStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onBeadStartCalls))
	}
	if obs2.onBeadStartCalls[0].ID != "bead-1" {
		t.Errorf("obs2: expected ID 'bead-1', got %q", obs2.onBeadStartCalls[0].ID)
	}
}

func TestMultiObserver_OnBeadComplete(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	result := BeadResult{
		Bead:     beads.Bead{ID: "bead-1", Title: "Test"},
		Outcome:  OutcomeSuccess,
		Duration: 5 * time.Second,
		ChatID:   "chat-123",
	}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnBeadComplete(result)

	if len(obs1.onBeadCompleteCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onBeadCompleteCalls))
	}
	if obs1.onBeadCompleteCalls[0].ChatID != "chat-123" {
		t.Errorf("obs1: expected ChatID 'chat-123', got %q", obs1.onBeadCompleteCalls[0].ChatID)
	}

	if len(obs2.onBeadCompleteCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onBeadCompleteCalls))
	}
	if obs2.onBeadCompleteCalls[0].ChatID != "chat-123" {
		t.Errorf("obs2: expected ChatID 'chat-123', got %q", obs2.onBeadCompleteCalls[0].ChatID)
	}
}

func TestMultiObserver_OnLoopEnd(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	result := &CoreResult{
		Outcome:    OutcomeSuccess,
		Iterations: 3,
		Duration:   10 * time.Minute,
	}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnLoopEnd(result)

	if len(obs1.onLoopEndCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopEndCalls))
	}
	if obs1.onLoopEndCalls[0].Iterations != 3 {
		t.Errorf("obs1: expected Iterations 3, got %d", obs1.onLoopEndCalls[0].Iterations)
	}

	if len(obs2.onLoopEndCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopEndCalls))
	}
	if obs2.onLoopEndCalls[0].Iterations != 3 {
		t.Errorf("obs2: expected Iterations 3, got %d", obs2.onLoopEndCalls[0].Iterations)
	}
}

func TestMultiObserver_OnIterationStart(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(obs1, obs2)
	multi.OnIterationStart(5)

	if len(obs1.onIterationStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onIterationStartCalls))
	}
	if obs1.onIterationStartCalls[0] != 5 {
		t.Errorf("obs1: expected iteration 5, got %d", obs1.onIterationStartCalls[0])
	}

	if len(obs2.onIterationStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onIterationStartCalls))
	}
	if obs2.onIterationStartCalls[0] != 5 {
		t.Errorf("obs2: expected iteration 5, got %d", obs2.onIterationStartCalls[0])
	}
}

func TestMultiObserver_OnToolStart(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	event := ToolEvent{
		ID:        "tool-1",
		Name:      "read",
		Started:   true,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"path": "test.go",
		},
	}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnToolStart(event)

	if len(obs1.onToolStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onToolStartCalls))
	}
	if obs1.onToolStartCalls[0].ID != "tool-1" {
		t.Errorf("obs1: expected ID 'tool-1', got %q", obs1.onToolStartCalls[0].ID)
	}

	if len(obs2.onToolStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onToolStartCalls))
	}
	if obs2.onToolStartCalls[0].ID != "tool-1" {
		t.Errorf("obs2: expected ID 'tool-1', got %q", obs2.onToolStartCalls[0].ID)
	}
}

func TestMultiObserver_OnToolEnd(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	event := ToolEvent{
		ID:        "tool-1",
		Name:      "read",
		Started:   false,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"path": "test.go",
		},
	}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnToolEnd(event)

	if len(obs1.onToolEndCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onToolEndCalls))
	}
	if obs1.onToolEndCalls[0].ID != "tool-1" {
		t.Errorf("obs1: expected ID 'tool-1', got %q", obs1.onToolEndCalls[0].ID)
	}

	if len(obs2.onToolEndCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onToolEndCalls))
	}
	if obs2.onToolEndCalls[0].ID != "tool-1" {
		t.Errorf("obs2: expected ID 'tool-1', got %q", obs2.onToolEndCalls[0].ID)
	}
}

func TestMultiObserver_EmptyObservers(t *testing.T) {
	multi := NewMultiObserver()
	// Should not panic
	multi.OnLoopStart("test")
	multi.OnBeadStart(beads.Bead{ID: "test"})
	multi.OnBeadComplete(BeadResult{})
	multi.OnLoopEnd(&CoreResult{})
	multi.OnToolStart(ToolEvent{})
	multi.OnToolEnd(ToolEvent{})
	multi.OnIterationStart(1)
	multi.OnVerifyStart("test")
	multi.OnVerifyEnd(VerifyResult{})
}

func TestMultiObserver_ImplementsInterface(t *testing.T) {
	// Compile-time check that MultiObserver implements ProgressObserver
	var _ ProgressObserver = (*MultiObserver)(nil)
}

// failingObserver panics when called to simulate a failing observer.
type failingObserver struct {
	panicked bool
}

func (f *failingObserver) OnLoopStart(rootBead string) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnBeadStart(bead beads.Bead) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnBeadComplete(result BeadResult) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnLoopEnd(result *CoreResult) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnToolStart(event ToolEvent) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnToolEnd(event ToolEvent) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnIterationStart(iteration int) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnVerifyStart(beadID string) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnVerifyEnd(result VerifyResult) {
	f.panicked = true
	panic("observer failed")
}

func TestMultiObserver_OneObserverFailingDoesNotBlockOthers(t *testing.T) {
	failing := &failingObserver{}
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(failing, obs1, obs2)

	// Test OnLoopStart
	multi.OnLoopStart("test-bead")
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onLoopStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopStartCalls))
	}
	if len(obs2.onLoopStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopStartCalls))
	}

	// Reset and test OnBeadStart
	failing.panicked = false
	obs1.onBeadStartCalls = nil
	obs2.onBeadStartCalls = nil
	bead := beads.Bead{ID: "bead-1", Title: "Test"}
	multi.OnBeadStart(bead)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onBeadStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onBeadStartCalls))
	}
	if len(obs2.onBeadStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onBeadStartCalls))
	}

	// Reset and test OnBeadComplete
	failing.panicked = false
	obs1.onBeadCompleteCalls = nil
	obs2.onBeadCompleteCalls = nil
	result := BeadResult{
		Bead:     beads.Bead{ID: "bead-1", Title: "Test"},
		Outcome:  OutcomeSuccess,
		Duration: 5 * time.Second,
		ChatID:   "chat-123",
	}
	multi.OnBeadComplete(result)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onBeadCompleteCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onBeadCompleteCalls))
	}
	if len(obs2.onBeadCompleteCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onBeadCompleteCalls))
	}

	// Reset and test OnLoopEnd
	failing.panicked = false
	obs1.onLoopEndCalls = nil
	obs2.onLoopEndCalls = nil
	coreResult := &CoreResult{
		Outcome:    OutcomeSuccess,
		Iterations: 3,
		Duration:   10 * time.Minute,
	}
	multi.OnLoopEnd(coreResult)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onLoopEndCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopEndCalls))
	}
	if len(obs2.onLoopEndCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopEndCalls))
	}

	// Reset and test OnIterationStart
	failing.panicked = false
	obs1.onIterationStartCalls = nil
	obs2.onIterationStartCalls = nil
	multi.OnIterationStart(3)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onIterationStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onIterationStartCalls))
	}
	if len(obs2.onIterationStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onIterationStartCalls))
	}

	// Reset and test OnToolStart
	failing.panicked = false
	obs1.onToolStartCalls = nil
	obs2.onToolStartCalls = nil
	toolEvent := ToolEvent{
		ID:        "tool-1",
		Name:      "read",
		Started:   true,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"path": "test.go",
		},
	}
	multi.OnToolStart(toolEvent)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onToolStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onToolStartCalls))
	}
	if len(obs2.onToolStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onToolStartCalls))
	}

	// Reset and test OnToolEnd
	failing.panicked = false
	obs1.onToolEndCalls = nil
	obs2.onToolEndCalls = nil
	toolEventEnd := ToolEvent{
		ID:        "tool-1",
		Name:      "read",
		Started:   false,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"path": "test.go",
		},
	}
	multi.OnToolEnd(toolEventEnd)
	if !failing.panicked {
		t.Error("failing observer should have been called")
	}
	if len(obs1.onToolEndCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onToolEndCalls))
	}
	if len(obs2.onToolEndCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onToolEndCalls))
	}
}
