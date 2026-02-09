package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devdeploy/internal/rules"
)

// setupWorktreeDir creates a temp dir simulating a worktree with a .git file
// pointing to a gitdir under the temp dir. Returns (worktreePath, gitDir).
func setupWorktreeDir(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	worktree := filepath.Join(base, "worktree")
	gitDir := filepath.Join(base, "gitdir")

	if err := os.MkdirAll(worktree, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Worktree .git file pointing to our gitDir.
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+gitDir+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return worktree, gitDir
}

func TestInjectWorktreeRules_CreatesRuleFiles(t *testing.T) {
	wt, _ := setupWorktreeDir(t)

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Verify rule files exist with correct content.
	for name, expected := range rules.Files() {
		path := filepath.Join(wt, ".cursor", "rules", name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("rule file %s not found: %v", name, err)
			continue
		}
		if string(got) != string(expected) {
			t.Errorf("rule file %s content mismatch (len got=%d want=%d)", name, len(got), len(expected))
		}
	}
}

func TestInjectWorktreeRules_CreatesDevLogDir(t *testing.T) {
	wt, _ := setupWorktreeDir(t)

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	devLog := filepath.Join(wt, "dev-log")
	info, err := os.Stat(devLog)
	if err != nil {
		t.Fatalf("dev-log dir not found: %v", err)
	}
	if !info.IsDir() {
		t.Error("dev-log is not a directory")
	}
}

func TestInjectWorktreeRules_AddsExcludeEntries(t *testing.T) {
	wt, gitDir := setupWorktreeDir(t)

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	excludePath := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("exclude file not found: %v", err)
	}
	content := string(data)
	for _, entry := range excludeEntries {
		if !strings.Contains(content, entry) {
			t.Errorf("exclude file missing entry %q", entry)
		}
	}
}

func TestInjectWorktreeRules_Idempotent(t *testing.T) {
	wt, gitDir := setupWorktreeDir(t)

	// Run twice.
	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("first inject: %v", err)
	}
	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("second inject: %v", err)
	}

	// Exclude entries should not be duplicated.
	excludePath := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("exclude file not found: %v", err)
	}
	for _, entry := range excludeEntries {
		count := strings.Count(string(data), entry)
		if count != 1 {
			t.Errorf("exclude entry %q appears %d times (want 1)", entry, count)
		}
	}

	// Rule files should still match embedded content.
	for name, expected := range rules.Files() {
		path := filepath.Join(wt, ".cursor", "rules", name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("rule file %s: %v", name, err)
			continue
		}
		if string(got) != string(expected) {
			t.Errorf("rule file %s content mismatch after second inject", name)
		}
	}
}

func TestInjectWorktreeRules_PreservesExistingExclude(t *testing.T) {
	wt, gitDir := setupWorktreeDir(t)

	// Pre-populate exclude with existing content.
	infoDir := filepath.Join(gitDir, "info")
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := "# existing exclude\n*.log\n"
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(infoDir, "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// Original content preserved.
	if !strings.Contains(content, "*.log") {
		t.Error("existing exclude content was lost")
	}
	// New entries added.
	for _, entry := range excludeEntries {
		if !strings.Contains(content, entry) {
			t.Errorf("missing exclude entry %q", entry)
		}
	}
}

func TestInjectWorktreeRules_RegularGitDir(t *testing.T) {
	// Test with a regular repo (where .git is a directory, not a file).
	base := t.TempDir()
	wt := filepath.Join(base, "repo")
	gitDir := filepath.Join(wt, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Exclude should be under .git/info/exclude.
	excludePath := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("exclude file not found: %v", err)
	}
	for _, entry := range excludeEntries {
		if !strings.Contains(string(data), entry) {
			t.Errorf("missing exclude entry %q", entry)
		}
	}
}

func TestInjectWorktreeRules_RelativeGitDir(t *testing.T) {
	// Test with a worktree .git file containing a relative path.
	base := t.TempDir()
	wt := filepath.Join(base, "worktree")
	gitDir := filepath.Join(base, "actual-gitdir")

	if err := os.MkdirAll(wt, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write .git with relative path.
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: ../actual-gitdir\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Exclude should be under the resolved gitdir.
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if _, err := os.ReadFile(excludePath); err != nil {
		t.Fatalf("exclude file not found (relative gitdir): %v", err)
	}
}
