package ralph

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// mockBDForStatus returns a bd.Runner that returns a bead with the given status.
func mockBDForStatus(id, status string) bd.Runner {
	return func(dir string, args ...string) ([]byte, error) {
		return json.Marshal([]bdShowReadyEntry{{
			bdShowBase: bdShowBase{
				ID:     id,
				Title:  "Test Bead",
				Status: status,
			},
			Priority:        2,
			DependencyCount: 0,
		}})
	}
}

// mockBDShowClosed returns a bd.Runner that returns closed after N calls.
func mockBDShowClosed(id string, closeAfter int) bd.Runner {
	callCount := 0
	return func(dir string, args ...string) ([]byte, error) {
		callCount++
		status := "open"
		if callCount > closeAfter {
			status = "closed"
		}
		return json.Marshal([]bdShowReadyEntry{{
			bdShowBase: bdShowBase{
				ID:     id,
				Title:  "Test Bead",
				Status: status,
			},
			Priority:        2,
			DependencyCount: 0,
		}})
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

func TestCore_Run_BeadAlreadyClosed(t *testing.T) {
	execute, prompts := mockExecute()

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "closed"),
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
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

	if result.Outcome != OutcomeSuccess {
		t.Errorf("expected success outcome, got %v", result.Outcome)
	}
	if result.Iterations != 0 {
		t.Errorf("expected 0 iterations (bead already closed), got %d", result.Iterations)
	}
	if len(*prompts) != 0 {
		t.Errorf("expected 0 execute calls, got %d", len(*prompts))
	}
}

func TestCore_Run_SingleIteration(t *testing.T) {
	execute, prompts := mockExecute()

	// Bead closes after first iteration
	runBD := mockBDShowClosed("test-bead", 2)

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         runBD,
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
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

	if result.Outcome != OutcomeSuccess {
		t.Errorf("expected success outcome, got %v", result.Outcome)
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if len(*prompts) != 1 {
		t.Errorf("expected 1 execute call, got %d", len(*prompts))
	}
}

func TestCore_Run_MaxIterations(t *testing.T) {
	execute, prompts := mockExecute()

	// Bead never closes
	runBD := mockBDForStatus("test-bead", "open")

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 3,
		Output:        &out,
		RunBD:         runBD,
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID, Title: "Test Bead"}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "mock prompt", nil
		},
		Execute:  execute,
		AssessFn: mockAssess(OutcomeFailure), // Agent fails to close bead
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeMaxIterations {
		t.Errorf("expected max-iterations outcome, got %v", result.Outcome)
	}
	if result.Iterations != 3 {
		t.Errorf("expected 3 iterations, got %d", result.Iterations)
	}
	if len(*prompts) != 3 {
		t.Errorf("expected 3 execute calls, got %d", len(*prompts))
	}
}

func TestCore_Run_QuestionStopsLoop(t *testing.T) {
	execute, prompts := mockExecute()

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "open"),
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID, Title: "Test Bead"}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "mock prompt", nil
		},
		Execute:  execute,
		AssessFn: mockAssess(OutcomeQuestion),
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeQuestion {
		t.Errorf("expected question outcome, got %v", result.Outcome)
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration (stop on question), got %d", result.Iterations)
	}
	if len(*prompts) != 1 {
		t.Errorf("expected 1 execute call, got %d", len(*prompts))
	}
}

func TestCore_Run_TimeoutStopsLoop(t *testing.T) {
	execute, _ := mockExecute()

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "open"),
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID, Title: "Test Bead"}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "mock prompt", nil
		},
		Execute:  execute,
		AssessFn: mockAssess(OutcomeTimeout),
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeTimeout {
		t.Errorf("expected timeout outcome, got %v", result.Outcome)
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration (stop on timeout), got %d", result.Iterations)
	}
}

func TestCore_Run_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "open"),
	}

	result, err := core.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeTimeout {
		t.Errorf("expected timeout outcome (context cancelled), got %v", result.Outcome)
	}
	if result.Iterations != 0 {
		t.Errorf("expected 0 iterations after cancellation, got %d", result.Iterations)
	}
}

// testObserver records all observer callbacks for verification.
type testObserver struct {
	NoopObserver
	loopStartCalls      int
	beadStartCalls      int
	beadCompleteCalls   int
	loopEndCalls        int
	iterationStartCalls int
	lastBeadResult      *BeadResult
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

func (o *testObserver) OnIterationStart(iteration int) {
	o.iterationStartCalls++
}

func TestCore_Run_Observer(t *testing.T) {
	observer := &testObserver{}
	execute, _ := mockExecute()

	// Bead closes after first iteration
	runBD := mockBDShowClosed("test-bead", 2)

	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		Observer:      observer,
		RunBD:         runBD,
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute:  execute,
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
	if observer.iterationStartCalls != 2 {
		t.Errorf("expected 2 OnIterationStart calls, got %d", observer.iterationStartCalls)
	}
	if observer.beadCompleteCalls != 1 {
		t.Errorf("expected 1 OnBeadComplete call, got %d", observer.beadCompleteCalls)
	}
}

func TestCore_Run_PromptError(t *testing.T) {
	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "open"),
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return nil, context.DeadlineExceeded
		},
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeFailure {
		t.Errorf("expected failure outcome, got %v", result.Outcome)
	}
}

func TestCore_Run_ExecuteError(t *testing.T) {
	var out bytes.Buffer
	core := &Core{
		WorkDir:       "/tmp/test",
		RootBead:      "test-bead",
		MaxIterations: 10,
		Output:        &out,
		RunBD:         mockBDForStatus("test-bead", "open"),
		FetchPrompt: func(runBD bd.Runner, workDir, beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID}, nil
		},
		Render: func(data *PromptData) (string, error) {
			return "prompt", nil
		},
		Execute: func(ctx context.Context, workDir, prompt string) (*AgentResult, error) {
			return nil, context.DeadlineExceeded
		},
	}

	result, err := core.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Outcome != OutcomeFailure {
		t.Errorf("expected failure outcome, got %v", result.Outcome)
	}
}

func TestNoopObserver(t *testing.T) {
	var obs NoopObserver
	obs.OnLoopStart("test")
	obs.OnBeadStart(beads.Bead{})
	obs.OnBeadComplete(BeadResult{})
	obs.OnLoopEnd(&CoreResult{})
	obs.OnToolStart(ToolEvent{})
	obs.OnToolEnd(ToolEvent{})
	obs.OnIterationStart(0)
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
