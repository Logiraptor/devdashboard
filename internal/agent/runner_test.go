package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"devdeploy/internal/progress"
)

func TestEmitAfter_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so emitAfter returns StatusAborted

	cmd := emitAfter(ctx, 100*time.Millisecond, progress.Event{
		Message: "Should not appear",
		Status:  progress.StatusDone,
	})
	msg := cmd()
	ev, ok := msg.(progress.Event)
	if !ok {
		t.Fatalf("expected progress.Event, got %T", msg)
	}
	if ev.Status != progress.StatusAborted {
		t.Errorf("expected StatusAborted when ctx cancelled, got %s", ev.Status)
	}
	if ev.Message != "Aborted" {
		t.Errorf("expected Message 'Aborted', got %q", ev.Message)
	}
}

func TestStubRunner_RunReturnsCmd(t *testing.T) {
	ctx := context.Background()
	projectDir := filepath.Join(t.TempDir(), "testproj")
	planPath := filepath.Join(projectDir, "plan.md")
	designPath := filepath.Join(projectDir, "design.md")

	runner := &StubRunner{}
	cmd := runner.Run(ctx, projectDir, planPath, designPath)
	if cmd == nil {
		t.Fatal("Run should return non-nil Cmd")
	}
	msg := cmd()
	if msg == nil {
		t.Error("Cmd should return non-nil Msg")
	}
}
