package ralph

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// mockBDForCore returns a BDRunner that returns beads one at a time.
// Each call returns the next bead until all are exhausted.
func mockBDForCore(beadsList []beads.Bead) BDRunner {
	var callCount int32
	return func(dir string, args ...string) ([]byte, error) {
		n := atomic.AddInt32(&callCount, 1)
		idx := int(n) - 1
		if idx < len(beadsList) {
			// Return one bead at a time
			b := beadsList[idx]
			entries := []bdReadyEntry{{
				ID:        b.ID,
				Title:     b.Title,
				Status:    b.Status,
				Priority:  b.Priority,
				Labels:    b.Labels,
				CreatedAt: b.CreatedAt,
			}}
			return json.Marshal(entries)
		}
		// All beads consumed
		return []byte("[]"), nil
	}
}

// mockExecute returns an Execute function that records calls and returns success.
func mockExecute() (func(ctx context.Context, workDir, prompt string) (*AgentResult, error), *[]string) {
	var prompts []string
	fn := func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
		prompts = append(prompts, prompt)
		return &AgentResult{
			ExitCode: 0,
			Duration: 100 * time.Millisecond,
		}, nil
	}
	return fn, &prompts
}

// mockAssess returns an AssessFn that always returns the given outcome.
func mockAssess(outcome Outcome) func(string, string, *AgentResult) (Outcome, string) {
	return func(workDir, beadID string, result *AgentResult) (Outcome, string) {
		return outcome, "mock assessment"
	}
}

func TestCore_Run_SingleBead_Success(t *testing.T) {
	bead := beads.Bead{ID: "test-bead", Title: "Test Bead", Status: "open", Priority: 2}
	execute, prompts := mockExecute()

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		RootBead:    "test-epic",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID, Title: "Test Bead"}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "mock prompt for " + data.ID, nil
		},
		Execute:  execute,
		AssessFn: mockAssess(OutcomeSuccess),
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", result.Failed)
	}
	if len(*prompts) != 1 {
		t.Errorf("expected 1 execute call, got %d", len(*prompts))
	}
}

func TestCore_Run_NoBeads(t *testing.T) {
	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{}),
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", result.Succeeded)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestCore_Run_MixedOutcomes(t *testing.T) {
	beadsList := []beads.Bead{
		{ID: "success-bead", Title: "Success", Status: "open", Priority: 1},
		{ID: "fail-bead", Title: "Fail", Status: "open", Priority: 2},
		{ID: "question-bead", Title: "Question", Status: "open", Priority: 3},
	}

	outcomeMap := map[string]Outcome{
		"success-bead":  OutcomeSuccess,
		"fail-bead":     OutcomeFailure,
		"question-bead": OutcomeQuestion,
	}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore(beadsList),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute: func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return &AgentResult{ExitCode: 0, Duration: 10 * time.Millisecond}, nil
		},
		AssessFn: func(workDir, beadID string, result *AgentResult) (Outcome, string) {
			return outcomeMap[beadID], "test"
		},
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if result.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", result.Failed)
	}
	if result.Questions != 1 {
		t.Errorf("expected 1 question, got %d", result.Questions)
	}
}

func TestCore_Run_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{{ID: "bead", Title: "Test"}}),
	}

	result, err := core.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return early with no work done
	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded after cancellation, got %d", result.Succeeded)
	}
}

// testObserver records all observer callbacks for verification.
type testObserver struct {
	NoopObserver
	loopStartCalls   int
	beadStartCalls   int
	beadCompleteCalls int
	loopEndCalls     int
	lastBeadResult   *BeadResult
}

func (o *testObserver) OnLoopStart(rootBead string) {
	o.loopStartCalls++
}

func (o *testObserver) OnBeadStart(bead beads.Bead) {
	o.beadStartCalls++
}

func (o *testObserver) OnBeadComplete(result BeadResult) {
	o.beadCompleteCalls++
	o.lastBeadResult = &result
}

func (o *testObserver) OnLoopEnd(result *CoreResult) {
	o.loopEndCalls++
}

func TestCore_Run_Observer(t *testing.T) {
	bead := beads.Bead{ID: "observed-bead", Title: "Observed", Status: "open"}
	observer := &testObserver{}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		Observer:    observer,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute: func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return &AgentResult{ExitCode: 0, Duration: 50 * time.Millisecond}, nil
		},
		AssessFn: mockAssess(OutcomeSuccess),
	}

	_, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if observer.loopStartCalls != 1 {
		t.Errorf("expected 1 OnLoopStart call, got %d", observer.loopStartCalls)
	}
	if observer.beadStartCalls != 1 {
		t.Errorf("expected 1 OnBeadStart call, got %d", observer.beadStartCalls)
	}
	if observer.beadCompleteCalls != 1 {
		t.Errorf("expected 1 OnBeadComplete call, got %d", observer.beadCompleteCalls)
	}
	if observer.loopEndCalls != 1 {
		t.Errorf("expected 1 OnLoopEnd call, got %d", observer.loopEndCalls)
	}
	if observer.lastBeadResult == nil {
		t.Fatal("expected lastBeadResult to be set")
	}
	if observer.lastBeadResult.Outcome != OutcomeSuccess {
		t.Errorf("expected success outcome, got %v", observer.lastBeadResult.Outcome)
	}
}

func TestCore_Run_FetchPromptError(t *testing.T) {
	bead := beads.Bead{ID: "error-bead", Title: "Error", Status: "open"}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return nil, context.DeadlineExceeded
		},
		AssessFn: mockAssess(OutcomeSuccess), // Won't be called
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fetch error should result in failure
	if result.Failed != 1 {
		t.Errorf("expected 1 failed (prompt fetch error), got %d", result.Failed)
	}
}

func TestCore_Run_RenderError(t *testing.T) {
	bead := beads.Bead{ID: "render-error", Title: "Render Error", Status: "open"}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "", context.DeadlineExceeded
		},
		AssessFn: mockAssess(OutcomeSuccess), // Won't be called
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Render error should result in failure
	if result.Failed != 1 {
		t.Errorf("expected 1 failed (render error), got %d", result.Failed)
	}
}

func TestCore_Run_ExecuteError(t *testing.T) {
	bead := beads.Bead{ID: "exec-error", Title: "Exec Error", Status: "open"}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute: func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return nil, context.DeadlineExceeded
		},
		AssessFn: mockAssess(OutcomeSuccess), // Won't be called
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Execute error should result in failure
	if result.Failed != 1 {
		t.Errorf("expected 1 failed (execute error), got %d", result.Failed)
	}
}

func TestCore_Run_Timeout(t *testing.T) {
	bead := beads.Bead{ID: "timeout-bead", Title: "Timeout", Status: "open"}

	var out bytes.Buffer
	core := &Core{
		WorkDir:     "/tmp/test",
		MaxParallel: 1,
		Output:      &out,
		RunBD:       mockBDForCore([]beads.Bead{bead}),
		FetchPrompt: func(runBD BDRunner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute: func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return &AgentResult{ExitCode: -1, Duration: 10 * time.Minute, TimedOut: true}, nil
		},
		AssessFn: func(workDir, beadID string, result *AgentResult) (Outcome, string) {
			if result.TimedOut {
				return OutcomeTimeout, "timed out"
			}
			return OutcomeSuccess, ""
		},
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TimedOut != 1 {
		t.Errorf("expected 1 timed out, got %d", result.TimedOut)
	}
}

func TestNoopObserver(t *testing.T) {
	// Verify NoopObserver implements all methods without panicking
	var obs NoopObserver
	obs.OnLoopStart("test")
	obs.OnBeadStart(beads.Bead{})
	obs.OnBeadComplete(BeadResult{})
	obs.OnLoopEnd(&CoreResult{})
	obs.OnToolStart(ToolEvent{})
	obs.OnToolEnd(ToolEvent{})
	// If we get here, the test passes
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
		{5 * time.Minute, "5m0s"},
		{65 * time.Minute, "1h5m"},
		{2*time.Hour + 30*time.Minute, "2h30m"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.d)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
