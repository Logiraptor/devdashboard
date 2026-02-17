package ralph

import (
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// mergeCall tracks a merge callback.
type mergeCall struct {
	BeadID     string
	BranchName string
	Success    bool
	ErrMsg     string
}

// multiTestObserver tracks method calls for testing.
type multiTestObserver struct {
	onLoopStartCalls     []string
	onBeadStartCalls     []beads.Bead
	onBeadCompleteCalls  []BeadResult
	onLoopEndCalls       []*CoreResult
	onToolStartCalls     []ToolEvent
	onToolEndCalls       []ToolEvent
	onMergeStartCalls    []mergeCall
	onMergeCompleteCalls []mergeCall
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

func (t *multiTestObserver) OnMergeStart(beadID, branchName string) {
	t.onMergeStartCalls = append(t.onMergeStartCalls, mergeCall{BeadID: beadID, BranchName: branchName})
}

func (t *multiTestObserver) OnMergeComplete(beadID string, success bool, errMsg string) {
	t.onMergeCompleteCalls = append(t.onMergeCompleteCalls, mergeCall{BeadID: beadID, Success: success, ErrMsg: errMsg})
}

func TestNewMultiObserver_FiltersNilObservers(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(obs1, nil, obs2, nil)

	if len(multi.observers) != 2 {
		t.Errorf("expected 2 observers, got %d", len(multi.observers))
	}
	if multi.observers[0] != obs1 {
		t.Error("first observer should be obs1")
	}
	if multi.observers[1] != obs2 {
		t.Error("second observer should be obs2")
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
	multi.OnLoopStart("test-epic")

	if len(obs1.onLoopStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopStartCalls))
	}
	if obs1.onLoopStartCalls[0] != "test-epic" {
		t.Errorf("obs1: expected 'test-epic', got %q", obs1.onLoopStartCalls[0])
	}

	if len(obs2.onLoopStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopStartCalls))
	}
	if obs2.onLoopStartCalls[0] != "test-epic" {
		t.Errorf("obs2: expected 'test-epic', got %q", obs2.onLoopStartCalls[0])
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
		Succeeded: 3,
		Failed:    2,
		Questions: 1,
		TimedOut:  0,
		Duration:  10 * time.Minute,
	}
	multi := NewMultiObserver(obs1, obs2)
	multi.OnLoopEnd(result)

	if len(obs1.onLoopEndCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopEndCalls))
	}
	if obs1.onLoopEndCalls[0].Succeeded != 3 {
		t.Errorf("obs1: expected Succeeded 3, got %d", obs1.onLoopEndCalls[0].Succeeded)
	}

	if len(obs2.onLoopEndCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopEndCalls))
	}
	if obs2.onLoopEndCalls[0].Succeeded != 3 {
		t.Errorf("obs2: expected Succeeded 3, got %d", obs2.onLoopEndCalls[0].Succeeded)
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

func TestMultiObserver_HandlesNilObserversInList(t *testing.T) {
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	// Create multi with nil in the middle
	multi := &MultiObserver{
		observers: []ProgressObserver{obs1, nil, obs2},
	}

	multi.OnLoopStart("test-epic")

	if len(obs1.onLoopStartCalls) != 1 {
		t.Errorf("obs1: expected 1 call, got %d", len(obs1.onLoopStartCalls))
	}
	if len(obs2.onLoopStartCalls) != 1 {
		t.Errorf("obs2: expected 1 call, got %d", len(obs2.onLoopStartCalls))
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
	multi.OnMergeStart("bead-1", "branch-1")
	multi.OnMergeComplete("bead-1", true, "")
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

func (f *failingObserver) OnMergeStart(beadID, branchName string) {
	f.panicked = true
	panic("observer failed")
}

func (f *failingObserver) OnMergeComplete(beadID string, success bool, errMsg string) {
	f.panicked = true
	panic("observer failed")
}

func TestMultiObserver_OneObserverFailingDoesNotBlockOthers(t *testing.T) {
	failing := &failingObserver{}
	obs1 := &multiTestObserver{}
	obs2 := &multiTestObserver{}

	multi := NewMultiObserver(failing, obs1, obs2)

	// Test OnLoopStart
	multi.OnLoopStart("test-epic")
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
		Succeeded: 3,
		Failed:    2,
		Questions: 1,
		TimedOut:  0,
		Duration:  10 * time.Minute,
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
