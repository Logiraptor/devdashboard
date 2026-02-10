package tmux

import (
	"os"
	"path/filepath"
	"testing"
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

func TestSplitPane_KillPane(t *testing.T) {
	skipIfTmuxTestsDisabled(t)
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
	skipIfTmuxTestsDisabled(t)
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	defer func() { _ = KillPane(paneID) }()
	if err := SendKeys(paneID, "echo ok\n"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
}

func TestSplitPane_InvalidDir(t *testing.T) {
	skipIfTmuxTestsDisabled(t)
	_, err := SplitPane(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
}

func TestBreakPane_JoinPane(t *testing.T) {
	skipIfTmuxTestsDisabled(t)
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	defer func() { _ = KillPane(paneID) }()
	if err := BreakPane(paneID); err != nil {
		t.Fatalf("BreakPane: %v", err)
	}
	if err := JoinPane(paneID); err != nil {
		t.Fatalf("JoinPane: %v", err)
	}
}

func TestListPaneIDs(t *testing.T) {
	skipIfTmuxTestsDisabled(t)
	// Create a pane to ensure we have at least one pane
	workDir := t.TempDir()
	paneID, err := SplitPane(workDir)
	if err != nil {
		t.Fatalf("SplitPane: %v", err)
	}
	defer func() { _ = KillPane(paneID) }()

	// List all pane IDs
	paneIDs, err := ListPaneIDs()
	if err != nil {
		t.Fatalf("ListPaneIDs: %v", err)
	}
	if len(paneIDs) == 0 {
		t.Error("ListPaneIDs: expected at least one pane")
	}
	// Verify our created pane is in the list
	if !paneIDs[paneID] {
		t.Errorf("ListPaneIDs: expected pane %s to be in the list", paneID)
	}
	// Verify pane IDs have the correct format (%N)
	for id := range paneIDs {
		if len(id) == 0 || id[0] != '%' {
			t.Errorf("ListPaneIDs: pane ID %q should start with %%", id)
		}
	}
}
