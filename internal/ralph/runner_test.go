package ralph

import (
	"context"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// fakeBatcher creates a BeadBatcher (iter.Seq) that yields predetermined batches.
func fakeBatcher(batches [][]beads.Bead) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		for _, batch := range batches {
			if !yield(batch) {
				return
			}
		}
	}
}

// TestRunner_Run tests the Run method with a fake batcher.
func TestRunner_Run(t *testing.T) {
	tests := []struct {
		name           string
		batches        [][]beads.Bead
		maxIterations  int
		ctxCancel      bool
		wantIterations int
		wantStopReason StopReason
	}{
		{
			name: "single batch with one bead",
			batches: [][]beads.Bead{
				{
					{ID: "test-1", Title: "Test Bead 1"},
				},
			},
			maxIterations:  0,
			wantIterations: 1,
			wantStopReason: StopNormal,
		},
		{
			name: "multiple batches",
			batches: [][]beads.Bead{
				{
					{ID: "test-1", Title: "Test Bead 1"},
				},
				{
					{ID: "test-2", Title: "Test Bead 2"},
				},
				{
					{ID: "test-3", Title: "Test Bead 3"},
				},
			},
			maxIterations:  0,
			wantIterations: 3,
			wantStopReason: StopNormal,
		},
		{
			name: "max batches limit",
			batches: [][]beads.Bead{
				{
					{ID: "test-1", Title: "Test Bead 1"},
				},
				{
					{ID: "test-2", Title: "Test Bead 2"},
				},
				{
					{ID: "test-3", Title: "Test Bead 3"},
				},
			},
			maxIterations:  2,
			wantIterations: 2,
			wantStopReason: StopMaxIterations,
		},
		{
			name: "empty batch",
			batches: [][]beads.Bead{
				{},
				{
					{ID: "test-1", Title: "Test Bead 1"},
				},
			},
			maxIterations:  0,
			wantIterations: 1,
			wantStopReason: StopNormal,
		},
		{
			name:           "no batches",
			batches:        [][]beads.Bead{},
			maxIterations:  0,
			wantIterations: 0,
			wantStopReason: StopNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batcher := fakeBatcher(tt.batches)

			// Create a mock fetch prompt that returns valid data
			mockFetchPrompt := func(beadID string) (*PromptData, error) {
				return &PromptData{
					ID:          beadID,
					Title:       "Test",
					Description: "Test description",
				}, nil
			}

			// Create a mock render that returns a simple prompt
			mockRender := func(data *PromptData) (string, error) {
				return "test prompt", nil
			}

			// Create a mock execute that returns success
			mockExecute := func(ctx context.Context, prompt string) (*AgentResult, error) {
				return &AgentResult{
					ExitCode: 0,
					Duration: 100 * time.Millisecond,
				}, nil
			}

			// Create a mock assess that returns success
			mockAssess := func(beadID string, result *AgentResult) (Outcome, string) {
				return OutcomeSuccess, "bead closed successfully"
			}

			cfg := LoopConfig{
				WorkDir:       "/tmp",
				MaxIterations: tt.maxIterations,
				FetchPrompt:   mockFetchPrompt,
				Render:        mockRender,
				Execute:       mockExecute,
				AssessFn:      mockAssess,
			}

			ctx := context.Background()
			if tt.ctxCancel {
				ctx, _ = context.WithCancel(ctx)
			}

			runner, err := NewRunner(batcher, cfg)
			if err != nil {
				t.Fatalf("NewRunner() error = %v, want nil", err)
			}
			summary, err := runner.Run(ctx)

			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}

			if summary.Iterations != tt.wantIterations {
				t.Errorf("summary.Iterations = %d, want %d", summary.Iterations, tt.wantIterations)
			}

			if summary.StopReason != tt.wantStopReason {
				t.Errorf("summary.StopReason = %v, want %v", summary.StopReason, tt.wantStopReason)
			}
		})
	}
}

// TestRunner_Run_ContextCancellation tests that Run respects context cancellation.
func TestRunner_Run_ContextCancellation(t *testing.T) {
	// Create a batcher that yields batches slowly
	batches := [][]beads.Bead{
		{{ID: "test-1", Title: "Test Bead 1"}},
		{{ID: "test-2", Title: "Test Bead 2"}},
	}

	batcher := fakeBatcher(batches)

	mockFetchPrompt := func(beadID string) (*PromptData, error) {
		return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
	}
	mockRender := func(data *PromptData) (string, error) {
		return "test prompt", nil
	}
	mockExecute := func(ctx context.Context, prompt string) (*AgentResult, error) {
		// Simulate work that respects context
		select {
		case <-ctx.Done():
			return &AgentResult{ExitCode: 1, TimedOut: true}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return &AgentResult{ExitCode: 0, Duration: 100 * time.Millisecond}, nil
		}
	}
	mockAssess := func(beadID string, result *AgentResult) (Outcome, string) {
		if result.TimedOut {
			return OutcomeTimeout, "timed out"
		}
		return OutcomeSuccess, "success"
	}

	cfg := LoopConfig{
		WorkDir:     "/tmp",
		FetchPrompt: mockFetchPrompt,
		Render:      mockRender,
		Execute:     mockExecute,
		AssessFn:    mockAssess,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	runner, err := NewRunner(batcher, cfg)
	if err != nil {
		t.Fatalf("NewRunner() error = %v, want nil", err)
	}
	summary, err := runner.Run(ctx)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	if summary.StopReason != StopContextCancelled {
		t.Errorf("summary.StopReason = %v, want %v", summary.StopReason, StopContextCancelled)
	}
}
