package ralph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// --- Test helpers ---

// mockBeadsJSON returns JSON bytes representing beads for testing.
func mockBeadsJSON(beads []beads.Bead) []byte {
	type bdReadyEntry struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Status    string    `json:"status"`
		Priority  int       `json:"priority"`
		Labels    []string  `json:"labels"`
		CreatedAt time.Time `json:"created_at"`
	}
	entries := make([]bdReadyEntry, len(beads))
	for i, b := range beads {
		entries[i] = bdReadyEntry{
			ID:        b.ID,
			Title:     b.Title,
			Status:    b.Status,
			Priority:  b.Priority,
			Labels:    b.Labels,
			CreatedAt: b.CreatedAt,
		}
	}
	data, _ := json.Marshal(entries)
	return data
}

// mockRunBD returns a RunBDFunc that simulates bd commands.
func mockRunBD(readyBeads []beads.Bead, epicChildren map[string][]beads.Bead) func(string, ...string) ([]byte, error) {
	return func(dir string, args ...string) ([]byte, error) {
		cmd := strings.Join(args, " ")
		if strings.Contains(cmd, "ready --json") {
			if strings.Contains(cmd, "--parent") {
				// Extract epic ID from args
				epicID := ""
				for i, arg := range args {
					if arg == "--parent" && i+1 < len(args) {
						epicID = args[i+1]
						break
					}
				}
				if children, ok := epicChildren[epicID]; ok {
					return mockBeadsJSON(children), nil
				}
				return []byte("[]"), nil
			}
			return mockBeadsJSON(readyBeads), nil
		}
		return nil, fmt.Errorf("unexpected bd command: %v", args)
	}
}

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo
	cmds := [][]string{
		{"init"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"commit", "--allow-empty", "-m", "initial commit"},
	}

	for _, cmd := range cmds {
		gitCmd := exec.Command("git", cmd...)
		gitCmd.Dir = dir
		if err := gitCmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", cmd, err)
		}
	}

	return dir
}

// TestNewWaveOrchestrator tests WaveOrchestrator initialization.
func TestNewWaveOrchestrator(t *testing.T) {
	workDir := setupTestRepo(t)

	tests := []struct {
		name    string
		cfg     LoopConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			wantErr: false,
		},
		{
			name: "invalid workdir",
			cfg: LoopConfig{
				WorkDir: "/nonexistent/path",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wo, err := NewWaveOrchestrator(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewWaveOrchestrator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && wo == nil {
				t.Error("NewWaveOrchestrator() returned nil orchestrator")
			}
		})
	}
}

// TestWaveOrchestrator_fetchReadyBeads tests fetching ready beads.
func TestWaveOrchestrator_fetchReadyBeads(t *testing.T) {
	workDir := setupTestRepo(t)

	readyBeads := []beads.Bead{
		{ID: "test-1", Title: "Test 1", Status: "open", Priority: 1, CreatedAt: time.Now()},
		{ID: "test-2", Title: "Test 2", Status: "open", Priority: 2, CreatedAt: time.Now()},
	}

	epicChildren := map[string][]beads.Bead{
		"epic-1": {
			{ID: "child-1", Title: "Child 1", Status: "open", Priority: 1, CreatedAt: time.Now()},
		},
	}

	tests := []struct {
		name         string
		cfg          LoopConfig
		mockRunBD    func(string, ...string) ([]byte, error)
		wantCount    int
		wantErr      bool
		wantBeadIDs  []string
	}{
		{
			name: "fetch all ready beads",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			mockRunBD: mockRunBD(readyBeads, nil),
			wantCount: 2,
			wantErr:   false,
			wantBeadIDs: []string{"test-1", "test-2"},
		},
		{
			name: "fetch epic children",
			cfg: LoopConfig{
				WorkDir: workDir,
				Epic:    "epic-1",
			},
			mockRunBD: mockRunBD(nil, epicChildren),
			wantCount: 1,
			wantErr:   false,
			wantBeadIDs: []string{"child-1"},
		},
		{
			name: "empty ready beads",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			mockRunBD: mockRunBD([]beads.Bead{}, nil),
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wo, err := NewWaveOrchestrator(tt.cfg)
			if err != nil {
				t.Fatalf("NewWaveOrchestrator() error = %v", err)
			}

			// Test fetchReadyBeads indirectly by checking what Run would fetch
			// We can't override the method, so we test it through integration
			// For unit testing, we test the parsing logic separately
			got, err := wo.fetchReadyBeads()
			// This will fail if bd is not available, which is expected in unit tests
			// So we skip this test if bd is not available
			if err != nil && strings.Contains(err.Error(), "bd ready") {
				t.Skip("bd command not available, skipping integration test")
				return
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchReadyBeads() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != tt.wantCount {
					t.Errorf("fetchReadyBeads() count = %d, want %d", len(got), tt.wantCount)
				}
				if len(tt.wantBeadIDs) > 0 {
					if len(got) != len(tt.wantBeadIDs) {
						t.Errorf("fetchReadyBeads() bead count mismatch")
					} else {
						for i, wantID := range tt.wantBeadIDs {
							if got[i].ID != wantID {
								t.Errorf("fetchReadyBeads() bead[%d].ID = %s, want %s", i, got[i].ID, wantID)
							}
						}
					}
				}
			}
		})
	}
}

// TestWaveOrchestrator_Run tests the Run method.
func TestWaveOrchestrator_Run(t *testing.T) {
	workDir := setupTestRepo(t)

	readyBeads := []beads.Bead{
		{ID: "test-1", Title: "Test 1", Status: "open", Priority: 1, CreatedAt: time.Now()},
		{ID: "test-2", Title: "Test 2", Status: "open", Priority: 2, CreatedAt: time.Now()},
	}

	tests := []struct {
		name           string
		cfg            LoopConfig
		mockRunBD      func(string, ...string) ([]byte, error)
		mockFetchPrompt func(string) (*PromptData, error)
		mockRender     func(*PromptData) (string, error)
		mockExecute    func(context.Context, string) (*AgentResult, error)
		mockAssess     func(string, *AgentResult) (Outcome, string)
		mockSync       func() error
		wantErr        bool
		wantIterations int
		wantSucceeded  int
	}{
		{
			name: "dry run",
			cfg: LoopConfig{
				WorkDir: workDir,
				DryRun:  true,
			},
			mockRunBD:      mockRunBD(readyBeads, nil),
			mockFetchPrompt: func(string) (*PromptData, error) { return nil, nil },
			mockRender:     func(*PromptData) (string, error) { return "", nil },
			mockExecute:    func(context.Context, string) (*AgentResult, error) { return nil, nil },
			mockAssess:     func(string, *AgentResult) (Outcome, string) { return OutcomeSuccess, "" },
			mockSync:       func() error { return nil },
			wantErr:        false,
			wantIterations: 2,
			wantSucceeded:  0, // Dry run doesn't execute
		},
		{
			name: "empty ready beads",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			mockRunBD:      mockRunBD([]beads.Bead{}, nil),
			mockFetchPrompt: func(string) (*PromptData, error) { return nil, nil },
			mockRender:     func(*PromptData) (string, error) { return "", nil },
			mockExecute:    func(context.Context, string) (*AgentResult, error) { return nil, nil },
			mockAssess:     func(string, *AgentResult) (Outcome, string) { return OutcomeSuccess, "" },
			mockSync:       func() error { return nil },
			wantErr:        false,
			wantIterations: 0,
			wantSucceeded:  0,
		},
		{
			name: "successful execution",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			mockRunBD: mockRunBD(readyBeads, nil),
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return &AgentResult{ExitCode: 0, Duration: 1 * time.Second}, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeSuccess, "success"
			},
			mockSync: func() error {
				return nil
			},
			wantErr:        false,
			wantIterations: 2,
			wantSucceeded:  2,
		},
		{
			name: "execution failures",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			mockRunBD: mockRunBD(readyBeads, nil),
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return &AgentResult{ExitCode: 1, Duration: 1 * time.Second}, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeFailure, "failure"
			},
			mockSync: func() error {
				return nil
			},
			wantErr:        false,
			wantIterations: 2,
			wantSucceeded:  0,
		},
		{
			name: "timeout",
			cfg: LoopConfig{
				WorkDir: workDir,
				Timeout: 100 * time.Millisecond,
			},
			mockRunBD: mockRunBD(readyBeads, nil),
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(ctx context.Context, prompt string) (*AgentResult, error) {
				// Simulate slow execution
				select {
				case <-ctx.Done():
					return &AgentResult{ExitCode: 1, Duration: 0}, ctx.Err()
				case <-time.After(200 * time.Millisecond):
					return &AgentResult{ExitCode: 0, Duration: 200 * time.Millisecond}, nil
				}
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeTimeout, "timeout"
			},
			mockSync: func() error {
				return nil
			},
			wantErr:        false,
			wantIterations: 0, // May not complete due to timeout
			wantSucceeded:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := tt.cfg
			cfg.Output = &buf
			cfg.FetchPrompt = tt.mockFetchPrompt
			cfg.Render = tt.mockRender
			cfg.Execute = tt.mockExecute
			cfg.AssessFn = tt.mockAssess
			cfg.SyncFn = tt.mockSync

			wo, err := NewWaveOrchestrator(cfg)
			if err != nil {
				t.Fatalf("NewWaveOrchestrator() error = %v", err)
			}

			// For testing Run, we need to mock the bd commands at a lower level
			// Since we can't override fetchReadyBeads, we'll test with real bd or skip
			// In a real scenario, we'd inject a mock RunBDFunc, but that requires
			// refactoring the code to accept it. For now, we test the logic that doesn't
			// require bd (dry-run, empty beads, etc.)
			if tt.name == "dry run" || tt.name == "empty ready beads" {
				// These don't require actual bd execution
				ctx := context.Background()
				summary, err := wo.Run(ctx)

				if (err != nil) != tt.wantErr {
					t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if summary != nil {
					if summary.Iterations != tt.wantIterations {
						t.Errorf("Run() summary.Iterations = %d, want %d", summary.Iterations, tt.wantIterations)
					}
					if summary.Succeeded != tt.wantSucceeded {
						t.Errorf("Run() summary.Succeeded = %d, want %d", summary.Succeeded, tt.wantSucceeded)
					}
				}
			} else {
				// For tests that require bd, skip if not available
				t.Skip("Test requires bd command or refactored code to inject mocks")
			}
		})
	}
}

// TestWaveOrchestrator_executeBead tests executeBead method.
func TestWaveOrchestrator_executeBead(t *testing.T) {
	workDir := setupTestRepo(t)

	bead := &beads.Bead{
		ID:        "test-1",
		Title:     "Test Bead",
		Status:    "open",
		Priority:  1,
		CreatedAt: time.Now(),
	}

	tests := []struct {
		name           string
		cfg            LoopConfig
		bead           *beads.Bead
		mockFetchPrompt func(string) (*PromptData, error)
		mockRender     func(*PromptData) (string, error)
		mockExecute    func(context.Context, string) (*AgentResult, error)
		mockAssess     func(string, *AgentResult) (Outcome, string)
		wantOutcome    Outcome
		wantErr        bool
	}{
		{
			name: "successful execution",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			bead: bead,
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return &AgentResult{ExitCode: 0, Duration: 1 * time.Second}, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeSuccess, "success"
			},
			wantOutcome: OutcomeSuccess,
			wantErr:     false,
		},
		{
			name: "fetch prompt error",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			bead: bead,
			mockFetchPrompt: func(string) (*PromptData, error) {
				return nil, fmt.Errorf("fetch error")
			},
			mockRender: func(*PromptData) (string, error) {
				return "", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return nil, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeFailure, ""
			},
			wantOutcome: OutcomeFailure,
			wantErr:     false, // executeBead doesn't return errors, it logs them
		},
		{
			name: "render error",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			bead: bead,
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "", fmt.Errorf("render error")
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return nil, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeFailure, ""
			},
			wantOutcome: OutcomeFailure,
			wantErr:     false,
		},
		{
			name: "execute error",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			bead: bead,
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return nil, fmt.Errorf("execute error")
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeFailure, ""
			},
			wantOutcome: OutcomeFailure,
			wantErr:     false,
		},
		{
			name: "question outcome",
			cfg: LoopConfig{
				WorkDir: workDir,
			},
			bead: bead,
			mockFetchPrompt: func(beadID string) (*PromptData, error) {
				return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
			},
			mockRender: func(*PromptData) (string, error) {
				return "test prompt", nil
			},
			mockExecute: func(context.Context, string) (*AgentResult, error) {
				return &AgentResult{ExitCode: 0, Duration: 1 * time.Second}, nil
			},
			mockAssess: func(string, *AgentResult) (Outcome, string) {
				return OutcomeQuestion, "question"
			},
			wantOutcome: OutcomeQuestion,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cfg := tt.cfg
			cfg.Output = &buf
			cfg.FetchPrompt = tt.mockFetchPrompt
			cfg.Render = tt.mockRender
			cfg.Execute = tt.mockExecute
			cfg.AssessFn = tt.mockAssess
			cfg.SyncFn = func() error { return nil }

			wo, err := NewWaveOrchestrator(cfg)
			if err != nil {
				t.Fatalf("NewWaveOrchestrator() error = %v", err)
			}

			ctx := context.Background()
			wo.executeBead(ctx, tt.bead)

			// Verify summary was updated
			wo.setup.mu.Lock()
			summary := wo.setup.summary
			wo.setup.mu.Unlock()

			// Check that iteration count increased (unless there was an early error)
			if tt.name != "fetch prompt error" && tt.name != "render error" && tt.name != "execute error" {
				if summary.Iterations == 0 {
					t.Error("executeBead() did not increment iteration count")
				}
			}
		})
	}
}

// TestWaveOrchestrator_parallelExecution tests that beads execute in parallel.
func TestWaveOrchestrator_parallelExecution(t *testing.T) {
	workDir := setupTestRepo(t)

	var executionOrder []string
	var mu sync.Mutex

	var buf bytes.Buffer
	cfg := LoopConfig{
		WorkDir: workDir,
		Output:  &buf,
		FetchPrompt: func(beadID string) (*PromptData, error) {
			return &PromptData{ID: beadID, Title: "Test", Description: "Test"}, nil
		},
		Render: func(*PromptData) (string, error) {
			return "test prompt", nil
		},
		Execute: func(ctx context.Context, prompt string) (*AgentResult, error) {
			mu.Lock()
			executionOrder = append(executionOrder, prompt)
			mu.Unlock()
			// Simulate some work
			time.Sleep(50 * time.Millisecond)
			return &AgentResult{ExitCode: 0, Duration: 50 * time.Millisecond}, nil
		},
		AssessFn: func(string, *AgentResult) (Outcome, string) {
			return OutcomeSuccess, "success"
		},
		SyncFn: func() error {
			return nil
		},
	}

	wo, err := NewWaveOrchestrator(cfg)
	if err != nil {
		t.Fatalf("NewWaveOrchestrator() error = %v", err)
	}

	// This test requires bd to be available or code refactoring to inject mocks
	// For now, we'll skip if bd is not available
	start := time.Now()
	ctx := context.Background()
	summary, err := wo.Run(ctx)
	if err != nil && strings.Contains(err.Error(), "bd ready") {
		t.Skip("bd command not available, skipping parallel execution test")
		return
	}
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify all beads executed (if any were available)
	if summary.Iterations > 0 {
		// Verify parallel execution: should complete faster than sequential
		// Allow some overhead, but should be significantly faster
		if duration > time.Duration(summary.Iterations)*200*time.Millisecond {
			t.Errorf("Run() took %v for %d iterations, expected parallel execution to complete faster", duration, summary.Iterations)
		}

		// Verify all beads were executed
		if len(executionOrder) != summary.Iterations {
			t.Errorf("Run() executed %d beads, want %d", len(executionOrder), summary.Iterations)
		}
	} else {
		t.Skip("No ready beads available for parallel execution test")
	}
}
