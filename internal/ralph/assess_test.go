package ralph

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// mockBDShow returns a BDShowFunc for testing. Returns canned JSON output.
func mockBDShow(entry *bdShowEntry) BDShowFunc {
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

	outcome, summary := Assess("/fake", "test-1", result, nil)

	if outcome != OutcomeTimeout {
		t.Errorf("expected OutcomeTimeout, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Success(t *testing.T) {
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "closed",
		},
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 2 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeSuccess {
		t.Errorf("expected OutcomeSuccess, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_QuestionCreated(t *testing.T) {
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "open",
		},
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

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeQuestion {
		t.Errorf("expected OutcomeQuestion, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_QuestionInDependents(t *testing.T) {
	// Question beads may also appear as dependents.
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "open",
		},
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

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeQuestion {
		t.Errorf("expected OutcomeQuestion, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_NonZeroExit(t *testing.T) {
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "open",
		},
	})

	result := &AgentResult{
		ExitCode: 1,
		Duration: 30 * time.Second,
	}

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	if summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_BeadStillOpen(t *testing.T) {
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "in_progress",
		},
	})

	result := &AgentResult{
		ExitCode: 0,
		Duration: 5 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_BDShowError(t *testing.T) {
	bdShow := mockBDShow(nil) // simulate bd error

	result := &AgentResult{
		ExitCode: 0,
		Duration: 1 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_Failure_InvalidJSON(t *testing.T) {
	bdShow := func(workDir, beadID string) ([]byte, error) {
		return []byte("not valid json"), nil
	}

	result := &AgentResult{
		ExitCode: 0,
		Duration: 1 * time.Minute,
	}

	outcome, summary := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure, got %v", outcome)
	}
	t.Logf("summary: %s", summary)
}

func TestAssess_ClosedQuestionNotCounted(t *testing.T) {
	// A needs-human dep that is already closed should not count as a question.
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "open",
		},
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

	outcome, _ := Assess("/fake", "test-1", result, bdShow)

	if outcome != OutcomeFailure {
		t.Errorf("expected OutcomeFailure (closed question should not count), got %v", outcome)
	}
}

func TestAssess_TimeoutTakesPriority(t *testing.T) {
	// Even if the bead is closed, timeout should take priority.
	bdShow := mockBDShow(&bdShowEntry{
		bdShowBase: bdShowBase{
			ID:     "test-1",
			Status: "closed",
		},
	})

	result := &AgentResult{
		ExitCode: -1,
		Duration: 10 * time.Minute,
		TimedOut: true,
	}

	outcome, _ := Assess("/fake", "test-1", result, bdShow)

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

func TestMarshalOutcome(t *testing.T) {
	tests := []struct {
		outcome Outcome
		want    string
	}{
		{OutcomeSuccess, `"success"`},
		{OutcomeQuestion, `"question"`},
		{OutcomeFailure, `"failure"`},
		{OutcomeTimeout, `"timeout"`},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.outcome)
		if err != nil {
			t.Errorf("json.Marshal(Outcome(%d)) error = %v", tt.outcome, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("json.Marshal(Outcome(%d)) = %q, want %q", tt.outcome, string(got), tt.want)
		}
	}
}

func TestUnmarshalOutcome(t *testing.T) {
	tests := []struct {
		json string
		want Outcome
	}{
		{`"success"`, OutcomeSuccess},
		{`"question"`, OutcomeQuestion},
		{`"failure"`, OutcomeFailure},
		{`"timeout"`, OutcomeTimeout},
	}
	for _, tt := range tests {
		var got Outcome
		err := json.Unmarshal([]byte(tt.json), &got)
		if err != nil {
			t.Errorf("json.Unmarshal(%q, &Outcome) error = %v", tt.json, err)
			continue
		}
		if got != tt.want {
			t.Errorf("json.Unmarshal(%q, &Outcome) = %v, want %v", tt.json, got, tt.want)
		}
	}
}

func TestUnmarshalOutcome_Invalid(t *testing.T) {
	tests := []string{
		`"invalid"`,
		`"unknown"`,
		`123`,
		`null`,
	}
	for _, tt := range tests {
		var got Outcome
		err := json.Unmarshal([]byte(tt), &got)
		if err == nil {
			t.Errorf("json.Unmarshal(%q, &Outcome) expected error, got nil", tt)
		}
	}
}

func TestMarshalUnmarshalOutcome_RoundTrip(t *testing.T) {
	tests := []Outcome{
		OutcomeSuccess,
		OutcomeQuestion,
		OutcomeFailure,
		OutcomeTimeout,
	}
	for _, want := range tests {
		data, err := json.Marshal(want)
		if err != nil {
			t.Errorf("json.Marshal(%v) error = %v", want, err)
			continue
		}
		var got Outcome
		if err := json.Unmarshal(data, &got); err != nil {
			t.Errorf("json.Unmarshal(%q, &Outcome) error = %v", string(data), err)
			continue
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}
