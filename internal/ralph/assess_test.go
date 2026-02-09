package ralph

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// mockBDShow replaces runBDShow for testing. Returns canned JSON output.
func mockBDShow(entry *bdShowEntry) func(string, string) ([]byte, error) {
	return func(workDir, beadID string) ([]byte, error) {
		if entry == nil {
			return nil, fmt.Errorf("bd: bead not found")
		}
		data, err := json.Marshal([]bdShowEntry{*entry})
		return data, err
	}
}

func TestAssess_Timeout(t *testing.T) {
	result := &AgentResult{
		ExitCode: -1,
		Duration: 10 * time.Minute,
		TimedOut: true,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeTimeout {
		t.Errorf("expected OutcomeTimeout, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Success(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "closed",
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 2 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeSuccess {
		t.Errorf("expected OutcomeSuccess, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_QuestionCreated(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "open",
		Dependencies: []bdShowDep{
			{
				ID:             "test-1.q1",
				Status:         "open",
				Labels:         []string{"needs-human"},
				DependencyType: "blocks",
			},
		},
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 3 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeQuestion {
		t.Errorf("expected OutcomeQuestion, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_QuestionInDependents(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	// Question beads may also appear as dependents.
	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "open",
		Dependents: []bdShowDep{
			{
				ID:             "test-1.q2",
				Status:         "open",
				Labels:         []string{"needs-human", "project:foo"},
				DependencyType: "blocks",
			},
		},
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 1 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeQuestion {
		t.Errorf("expected OutcomeQuestion, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_NonZeroExit(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "open",
	})

	result := &AgentResult{
		ExitCode: 1,
		Duration: 30 * time.Second,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_BeadStillOpen(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "in_progress",
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 5 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_BDShowError(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = mockBDShow(nil) // simulate bd error

	result := &AgentResult{
		ExitCode: 0,
		Duration: 1 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_InvalidJSON(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	runBDShow = func(workDir, beadID string) ([]byte, error) {
		return []byte("not valid json"), nil
	}

	result := &AgentResult{
		ExitCode: 0,
		Duration: 1 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_ClosedQuestionNotCounted(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	// A needs-human dep that is already closed should not count as a question.
	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "open",
		Dependencies: []bdShowDep{
			{
				ID:             "test-1.q1",
				Status:         "closed",
				Labels:         []string{"needs-human"},
				DependencyType: "blocks",
			},
		},
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 2 * time.Minute,
	}

	outcome, _ := Assess("/fake", "test-1", result)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure (closed question should not count), got %v", outcome)
	}
}

func TestAssess_TimeoutTakesPriority(t *testing.T) {
	old := runBDShow
	defer func() { runBDShow = old }()

	// Even if the bead is closed, timeout should take priority.
	runBDShow = mockBDShow(&bdShowEntry{
		ID:     "test-1",
		Status: "closed",
	})

	result := &AgentResult{
		ExitCode: -1,
		Duration: 10 * time.Minute,
		TimedOut: true,
	}

	outcome, _ := Assess("/fake", "test-1", result)

	if outcome != OutcomeTimeout {
		t.Errorf("expected OutcomeTimeout to take priority, got %v", outcome)
	}
}

func TestOutcome_String(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    string
	}{
		{OutcomeSuccess, "success"},
		{OutcomeQuestion, "question"},
		{OutcomeFailure, "failure"},
		{OutcomeTimeout, "timeout"},
		{Outcome(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.outcome.String(); got != tt.want {
			t.Errorf("Outcome(%d).String() = %q, want %q", tt.outcome, got, tt.want)
		}
	}
}
