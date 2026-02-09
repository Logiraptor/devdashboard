package ralph

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// --- Test helpers ---

// beadQueue returns a PickNext function that yields beads in order, then nil.
func beadQueue(bb ...*beads.Bead) func() (*beads.Bead, error) {
	idx := 0
	return func() (*beads.Bead, error) {
		if idx >= len(bb) {
			return nil, nil
		}
		b := bb[idx]
		idx++
		return b, nil
	}
}

// staticPrompt returns a FetchPrompt that always returns the same PromptData.
func staticPrompt() func(string) (*PromptData, error) {
	return func(beadID string) (*PromptData, error) {
		return &PromptData{
			ID:          beadID,
			Title:       "Test bead",
			Description: "Test description",
		}, nil
	}
}

// staticRender returns a Render that returns a fixed string.
func staticRender() func(*PromptData) (string, error) {
	return func(data *PromptData) (string, error) {
		return fmt.Sprintf("prompt for %s", data.ID), nil
	}
}

// staticExecute returns an Execute that always returns a successful agent result.
func staticExecute() func(context.Context, string) (*AgentResult, error) {
	return func(ctx context.Context, prompt string) (*AgentResult, error) {
		return &AgentResult{
			ExitCode: 0,
			Duration: 1 * time.Second,
		}, nil
	}
}

// outcomeSequence returns an AssessFn that yields outcomes in order.
func outcomeSequence(outcomes ...Outcome) func(string, *AgentResult) (Outcome, string) {
	idx := 0
	return func(beadID string, result *AgentResult) (Outcome, string) {
		if idx >= len(outcomes) {
			return OutcomeFailure, "exhausted outcomes"
		}
		o := outcomes[idx]
		idx++
		return o, fmt.Sprintf("%s: %s", beadID, o)
	}
}

// noopSync returns a SyncFn that always succeeds.
func noopSync() func() error {
	return func() error { return nil }
}

// makeBead creates a simple test bead.
func makeBead(id string, priority int) *beads.Bead {
	return &beads.Bead{
		ID:        id,
		Title:     "Bead " + id,
		Status:    "open",
		Priority:  priority,
		CreatedAt: time.Now(),
	}
}

// baseCfg returns a LoopConfig with all test hooks wired up and output captured.
func baseCfg(buf *bytes.Buffer) LoopConfig {
	return LoopConfig{
		WorkDir:       "/fake/dir",
		MaxIterations: 20,
		Output:        buf,
		FetchPrompt:   staticPrompt(),
		Render:        staticRender(),
		Execute:       staticExecute(),
		SyncFn:        noopSync(),
	}
}

// --- Tests ---

func TestRun_SingleBeadSuccess(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(makeBead("b-1", 1))
	cfg.AssessFn = outcomeSequence(OutcomeSuccess)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", summary.Iterations)
	}
	if summary.Succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", summary.Succeeded)
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0", summary.Failed)
	}

	// Verify output contains the final summary.
	output := buf.String()
	if !strings.Contains(output, "1 succeeded") {
		t.Errorf("output missing success count:\n%s", output)
	}
}

func TestRun_MultipleBeadsAllSuccess(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(
		makeBead("b-1", 1),
		makeBead("b-2", 2),
		makeBead("b-3", 3),
	)
	cfg.AssessFn = outcomeSequence(OutcomeSuccess, OutcomeSuccess, OutcomeSuccess)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", summary.Iterations)
	}
	if summary.Succeeded != 3 {
		t.Errorf("succeeded = %d, want 3", summary.Succeeded)
	}
}

func TestRun_NoBeadsAvailable(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue() // empty queue

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 0 {
		t.Errorf("iterations = %d, want 0", summary.Iterations)
	}

	output := buf.String()
	if !strings.Contains(output, "no ready beads") {
		t.Errorf("output missing 'no ready beads':\n%s", output)
	}
}

func TestRun_ConsecutiveFailuresStopLoop(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(
		makeBead("b-1", 1),
		makeBead("b-2", 1),
		makeBead("b-3", 1),
		makeBead("b-4", 1), // should not be reached
	)
	cfg.AssessFn = outcomeSequence(OutcomeFailure, OutcomeFailure, OutcomeFailure)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", summary.Iterations)
	}
	if summary.Failed != 3 {
		t.Errorf("failed = %d, want 3", summary.Failed)
	}

	output := buf.String()
	if !strings.Contains(output, "too many consecutive failures") {
		t.Errorf("output missing consecutive failure message:\n%s", output)
	}
}

func TestRun_QuestionResetsConsecutiveFailures(t *testing.T) {
	// Sequence: fail, fail, question, fail, fail, fail -> stops at 3 consecutive.
	// The question in the middle resets the counter, so we get 6 iterations
	// instead of stopping at 3.
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(
		makeBead("b-1", 1),
		makeBead("b-2", 1),
		makeBead("b-3", 1), // question resets counter
		makeBead("b-4", 1),
		makeBead("b-5", 1),
		makeBead("b-6", 1), // 3rd consecutive failure -> stop
		makeBead("b-7", 1), // should not be reached
	)
	cfg.AssessFn = outcomeSequence(
		OutcomeFailure,  // consec=1
		OutcomeFailure,  // consec=2
		OutcomeQuestion, // consec=0
		OutcomeFailure,  // consec=1
		OutcomeFailure,  // consec=2
		OutcomeFailure,  // consec=3 -> stop
	)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 6 {
		t.Errorf("iterations = %d, want 6", summary.Iterations)
	}
	if summary.Failed != 5 {
		t.Errorf("failed = %d, want 5", summary.Failed)
	}
	if summary.Questions != 1 {
		t.Errorf("questions = %d, want 1", summary.Questions)
	}
}

func TestRun_TimeoutCountsAsFailure(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(
		makeBead("b-1", 1),
		makeBead("b-2", 1),
		makeBead("b-3", 1),
	)
	cfg.AssessFn = outcomeSequence(OutcomeTimeout, OutcomeTimeout, OutcomeTimeout)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", summary.Iterations)
	}
	if summary.TimedOut != 3 {
		t.Errorf("timed_out = %d, want 3", summary.TimedOut)
	}

	output := buf.String()
	if !strings.Contains(output, "too many consecutive failures") {
		t.Errorf("output missing consecutive failure message:\n%s", output)
	}
}

func TestRun_MaxIterationsCap(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.MaxIterations = 3

	// Infinite bead supply.
	cfg.PickNext = func() (*beads.Bead, error) {
		return makeBead("infinite", 1), nil
	}
	cfg.AssessFn = func(beadID string, result *AgentResult) (Outcome, string) {
		return OutcomeSuccess, "ok"
	}

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 3 {
		t.Errorf("iterations = %d, want 3 (max)", summary.Iterations)
	}
	if summary.Succeeded != 3 {
		t.Errorf("succeeded = %d, want 3", summary.Succeeded)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = func() (*beads.Bead, error) {
		return makeBead("never", 1), nil
	}

	summary, err := Run(ctx, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 0 {
		t.Errorf("iterations = %d, want 0 (context cancelled)", summary.Iterations)
	}

	output := buf.String()
	if !strings.Contains(output, "context cancelled") {
		t.Errorf("output missing context cancelled message:\n%s", output)
	}
}

func TestRun_DryRunMode(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.DryRun = true
	cfg.PickNext = beadQueue(makeBead("dry-1", 2))

	// Execute should never be called in dry-run.
	cfg.Execute = func(ctx context.Context, prompt string) (*AgentResult, error) {
		t.Fatal("Execute should not be called in dry-run mode")
		return nil, nil
	}

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 1 {
		t.Errorf("iterations = %d, want 1", summary.Iterations)
	}
	if summary.Succeeded != 0 {
		t.Errorf("succeeded = %d, want 0 (dry-run)", summary.Succeeded)
	}

	output := buf.String()
	if !strings.Contains(output, "[dry-run]") {
		t.Errorf("output missing dry-run marker:\n%s", output)
	}
	if !strings.Contains(output, "dry-1") {
		t.Errorf("output missing bead ID:\n%s", output)
	}
}

func TestRun_DryRunNoBeads(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.DryRun = true
	cfg.PickNext = beadQueue() // no beads

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 0 {
		t.Errorf("iterations = %d, want 0", summary.Iterations)
	}

	output := buf.String()
	if !strings.Contains(output, "no ready beads") {
		t.Errorf("output missing 'no ready beads':\n%s", output)
	}
}

func TestRun_PickerError(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = func() (*beads.Bead, error) {
		return nil, fmt.Errorf("bd not found")
	}

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when picker fails, got nil")
	}
	if !strings.Contains(err.Error(), "picking bead") {
		t.Errorf("error = %q, want to contain 'picking bead'", err.Error())
	}
}

func TestRun_ExecuteError(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(makeBead("b-1", 1))
	cfg.Execute = func(ctx context.Context, prompt string) (*AgentResult, error) {
		return nil, fmt.Errorf("agent binary not found")
	}

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when execute fails, got nil")
	}
	if !strings.Contains(err.Error(), "running agent") {
		t.Errorf("error = %q, want to contain 'running agent'", err.Error())
	}
}

func TestRun_FetchPromptError(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(makeBead("b-1", 1))
	cfg.FetchPrompt = func(beadID string) (*PromptData, error) {
		return nil, fmt.Errorf("bd show failed")
	}

	_, err := Run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when fetch fails, got nil")
	}
	if !strings.Contains(err.Error(), "fetching prompt data") {
		t.Errorf("error = %q, want to contain 'fetching prompt data'", err.Error())
	}
}

func TestRun_SyncErrorNonFatal(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.Verbose = true
	cfg.PickNext = beadQueue(makeBead("b-1", 1))
	cfg.AssessFn = outcomeSequence(OutcomeSuccess)
	cfg.SyncFn = func() error {
		return fmt.Errorf("bd sync: network error")
	}

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("sync error should be non-fatal: %v", err)
	}
	if summary.Succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", summary.Succeeded)
	}

	output := buf.String()
	if !strings.Contains(output, "bd sync warning") {
		t.Errorf("output missing sync warning:\n%s", output)
	}
}

func TestRun_VerboseLogging(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.Verbose = true
	cfg.PickNext = beadQueue(makeBead("verbose-1", 1))
	cfg.AssessFn = outcomeSequence(OutcomeSuccess)

	_, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "picked verbose-1") {
		t.Errorf("verbose output missing picked message:\n%s", output)
	}
}

func TestRun_MixedOutcomes(t *testing.T) {
	var buf bytes.Buffer
	cfg := baseCfg(&buf)
	cfg.PickNext = beadQueue(
		makeBead("b-1", 1),
		makeBead("b-2", 1),
		makeBead("b-3", 1),
		makeBead("b-4", 1),
	)
	cfg.AssessFn = outcomeSequence(
		OutcomeSuccess,
		OutcomeQuestion,
		OutcomeFailure,
		OutcomeSuccess,
	)

	summary, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if summary.Iterations != 4 {
		t.Errorf("iterations = %d, want 4", summary.Iterations)
	}
	if summary.Succeeded != 2 {
		t.Errorf("succeeded = %d, want 2", summary.Succeeded)
	}
	if summary.Questions != 1 {
		t.Errorf("questions = %d, want 1", summary.Questions)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
}

func TestRunSummary_ZeroValue(t *testing.T) {
	s := &RunSummary{}
	if s.Iterations != 0 || s.Succeeded != 0 || s.Questions != 0 ||
		s.Failed != 0 || s.TimedOut != 0 {
		t.Errorf("zero-value RunSummary should have all zeros: %+v", s)
	}
}
