package agent

import (
	"context"
	"path/filepath"
	"testing"
)

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
