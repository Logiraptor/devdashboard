package tmux

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// skipIfTmuxTestsDisabled skips the test unless DEVDEPLOY_TMUX_TESTS=1 is set.
// This prevents tests from polluting the user's live tmux session when running
// tests inside tmux. Tests should only run when explicitly enabled.
func skipIfTmuxTestsDisabled(t *testing.T) {
	if os.Getenv("DEVDEPLOY_TMUX_TESTS") != "1" {
		t.Skip("Skipping tmux test: set DEVDEPLOY_TMUX_TESTS=1 to enable")
	}
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
}

func TestEnsureSessionLifecycleAndCapturePane(t *testing.T) {
	skipIfTmuxTestsDisabled(t)
	workDir := t.TempDir()
	sessionName := fmt.Sprintf("dd-test-%d", time.Now().UnixNano())
	created, err := EnsureSession(sessionName, workDir, os.Getenv("SHELL"))
	if err != nil {
		t.Fatalf("EnsureSession(create): %v", err)
	}
	if !created {
		t.Fatal("expected EnsureSession to create a new session")
	}
	t.Cleanup(func() {
		_ = KillSession(sessionName)
	})

	exists, err := SessionExists(sessionName)
	if err != nil {
		t.Fatalf("SessionExists(after create): %v", err)
	}
	if !exists {
		t.Fatalf("expected session %q to exist", sessionName)
	}

	created, err = EnsureSession(sessionName, workDir, os.Getenv("SHELL"))
	if err != nil {
		t.Fatalf("EnsureSession(idempotent): %v", err)
	}
	if created {
		t.Fatal("expected EnsureSession to be idempotent")
	}

	if _, err := CapturePane(sessionName, CaptureOpts{LastLines: 20}); err != nil {
		t.Fatalf("CapturePane: %v", err)
	}

	if err := KillSession(sessionName); err != nil {
		t.Fatalf("KillSession: %v", err)
	}
	exists, err = SessionExists(sessionName)
	if err != nil {
		t.Fatalf("SessionExists(after kill): %v", err)
	}
	if exists {
		t.Fatalf("expected session %q to be killed", sessionName)
	}
}
