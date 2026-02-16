package ralph

import (
	"testing"
	"time"

	"devdeploy/internal/beads"
)

func TestTracingObserver_FullLifecycle(t *testing.T) {
	obs := NewTracingObserver()

	// Start loop
	obs.OnLoopStart("test-epic")

	if obs.traceID == "" {
		t.Error("traceID should be set after OnLoopStart")
	}
	if obs.loopSpanID == "" {
		t.Error("loopSpanID should be set after OnLoopStart")
	}

	// Start bead
	bead := beads.Bead{ID: "test-1", Title: "Test bead"}
	obs.OnBeadStart(bead)

	if obs.iterSpanID == "" {
		t.Error("iterSpanID should be set after OnBeadStart")
	}
	if obs.iterNum != 1 {
		t.Errorf("iterNum = %d, want 1", obs.iterNum)
	}

	// Tool calls
	toolEvent := ToolEvent{
		ID:        "tool-1",
		Name:      "shell",
		Started:   true,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"command": "echo hello",
		},
	}
	obs.OnToolStart(toolEvent)

	if _, ok := obs.toolSpans["tool-1"]; !ok {
		t.Error("tool span should be tracked after OnToolStart")
	}

	toolEvent.Started = false
	obs.OnToolEnd(toolEvent)

	if _, ok := obs.toolSpans["tool-1"]; ok {
		t.Error("tool span should be removed after OnToolEnd")
	}

	// Complete bead
	result := BeadResult{
		Bead:     bead,
		Outcome:  OutcomeSuccess,
		Duration: 5 * time.Second,
		ChatID:   "chat-123",
	}
	obs.OnBeadComplete(result)

	if obs.iterSpanID != "" {
		t.Error("iterSpanID should be cleared after OnBeadComplete")
	}

	// End loop
	coreResult := &CoreResult{
		Succeeded: 1,
		Failed:    0,
		Questions: 0,
		TimedOut:  0,
		Duration:  10 * time.Second,
	}
	obs.OnLoopEnd(coreResult)

	if obs.traceID != "" {
		t.Error("traceID should be cleared after OnLoopEnd")
	}
	if obs.loopSpanID != "" {
		t.Error("loopSpanID should be cleared after OnLoopEnd")
	}
}

func TestTracingObserver_MultipleIterations(t *testing.T) {
	obs := NewTracingObserver()

	obs.OnLoopStart("test-epic")

	// First iteration
	bead1 := beads.Bead{ID: "test-1", Title: "First bead"}
	obs.OnBeadStart(bead1)
	obs.OnBeadComplete(BeadResult{Bead: bead1, Outcome: OutcomeSuccess})

	if obs.iterNum != 1 {
		t.Errorf("iterNum = %d, want 1", obs.iterNum)
	}

	// Second iteration
	bead2 := beads.Bead{ID: "test-2", Title: "Second bead"}
	obs.OnBeadStart(bead2)
	obs.OnBeadComplete(BeadResult{Bead: bead2, Outcome: OutcomeFailure, ExitCode: 1})

	if obs.iterNum != 2 {
		t.Errorf("iterNum = %d, want 2", obs.iterNum)
	}

	obs.OnLoopEnd(&CoreResult{Succeeded: 1, Failed: 1})
}

func TestTracingObserver_ToolWithoutIteration(t *testing.T) {
	obs := NewTracingObserver()

	obs.OnLoopStart("test-epic")

	// Tool call without an active iteration - should use loop span as parent
	toolEvent := ToolEvent{
		ID:        "tool-1",
		Name:      "read",
		Started:   true,
		Timestamp: time.Now(),
	}
	obs.OnToolStart(toolEvent)

	if _, ok := obs.toolSpans["tool-1"]; !ok {
		t.Error("tool span should be tracked even without active iteration")
	}

	obs.OnToolEnd(toolEvent)
	obs.OnLoopEnd(&CoreResult{})
}
