package tmux

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitPane_KillPane(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	if paneID == "" {
		t.Fatal("SplitPane returned empty pane ID")
	}
	if err := KillPane(paneID); err != nil {
		t.Fatalf("KillPane: %v", err)
	}
}

func TestSendKeys(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	defer KillPane(paneID)
	if err := SendKeys(paneID, "echo ok\n"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
}

func TestSplitPane_InvalidDir(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	_, err := SplitPane(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestBreakPane_JoinPane(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	defer KillPane(paneID)
	if err := BreakPane(paneID); err != nil {
		t.Fatalf("BreakPane: %v", err)
	}
	if err := JoinPane(paneID); err != nil {
		t.Fatalf("JoinPane: %v", err)
	}
}

func TestWindowPaneCount(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	n, err := WindowPaneCount()
	if err != nil {
		t.Fatalf("WindowPaneCount: %v", err)
	}
	if n < 1 {
		t.Errorf("WindowPaneCount: expected >= 1, got %d", n)
	}
}

func TestEnsureLayout(t *testing.T) {
	if os.Getenv("TMUX") == "" {
		t.Skip("Skipping tmux test: not running inside tmux")
	}
	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	n, err := WindowPaneCount()
	if err != nil {
		t.Fatalf("WindowPaneCount after EnsureLayout: %v", err)
	}
	if n < 2 {
		t.Errorf("EnsureLayout: expected >= 2 panes, got %d", n)
	}
	// Idempotent: second call should be a no-op
	if err := EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout (second call): %v", err)
	}
}
