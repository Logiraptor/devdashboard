package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devdeploy/internal/rules"
)

// setupWorktreeDir creates a temp dir simulating a git worktree layout:
//
//	base/main-repo/.git/                         (common git dir)
//	base/main-repo/.git/worktrees/wt/            (per-worktree gitdir)
//	base/main-repo/.git/worktrees/wt/commondir   (relative path to common)
//	base/worktree/.git                            (file: "gitdir: ...")
//
// Returns (worktreePath, commonGitDir).
func setupWorktreeDir(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()

	worktree := filepath.Join(base, "worktree")
	commonDir := filepath.Join(base, "main-repo", ".git")
	wtGitDir := filepath.Join(commonDir, "worktrees", "wt")

	if err := os.MkdirAll(worktree, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtGitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Worktree .git file pointing to the per-worktree gitdir.
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// commondir file: relative path from per-worktree gitdir to common dir.
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return worktree, commonDir
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

func TestInjectWorktreeRules_AddsExcludeToCommonDir(t *testing.T) {
	wt, commonDir := setupWorktreeDir(t)

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Exclude must be in the common git dir, not the per-worktree gitdir.
	excludePath := filepath.Join(commonDir, "info", "exclude")
	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("exclude file not found at common dir: %v", err)
	}
	content := string(data)
	for _, entry := range excludeEntries {
		if !strings.Contains(content, entry) {
			t.Errorf("exclude file missing entry %q", entry)
		}
	}
}

func TestInjectWorktreeRules_Idempotent(t *testing.T) {
	wt, commonDir := setupWorktreeDir(t)

	// Run twice.
	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("first inject: %v", err)
	}
	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("second inject: %v", err)
	}

	// Exclude entries should not be duplicated.
	excludePath := filepath.Join(commonDir, "info", "exclude")
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
	wt, commonDir := setupWorktreeDir(t)

	// Pre-populate exclude with existing content.
	infoDir := filepath.Join(commonDir, "info")
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
	// The common dir IS the .git/ directory itself.
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
	// Test with a worktree .git file containing a relative gitdir path
	// and a commondir file pointing to the shared git directory.
	base := t.TempDir()
	wt := filepath.Join(base, "worktree")
	commonDir := filepath.Join(base, "main-repo", ".git")
	wtGitDir := filepath.Join(commonDir, "worktrees", "wt")

	if err := os.MkdirAll(wt, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtGitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write .git with relative path to the per-worktree gitdir.
	relGitDir, err := filepath.Rel(wt, wtGitDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+relGitDir+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// commondir: relative path from per-worktree gitdir to common dir.
	if err := os.WriteFile(filepath.Join(wtGitDir, "commondir"), []byte("../..\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Exclude should be in the common dir, not the per-worktree gitdir.
	excludePath := filepath.Join(commonDir, "info", "exclude")
	if _, err := os.ReadFile(excludePath); err != nil {
		t.Fatalf("exclude file not found in common dir: %v", err)
	}

	// Per-worktree gitdir should NOT have an exclude file.
	wtExclude := filepath.Join(wtGitDir, "info", "exclude")
	if _, err := os.Stat(wtExclude); !os.IsNotExist(err) {
		t.Errorf("exclude file should NOT exist in per-worktree gitdir, but it does")
	}
}

func TestInjectWorktreeRules_NoCommonDirFile(t *testing.T) {
	// Worktree without a commondir file — should fall back to the
	// per-worktree gitdir (defensive, shouldn't happen for real worktrees).
	base := t.TempDir()
	wt := filepath.Join(base, "worktree")
	gitDir := filepath.Join(base, "gitdir")

	if err := os.MkdirAll(wt, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// .git file without a commondir file in the target.
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+gitDir+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := InjectWorktreeRules(wt); err != nil {
		t.Fatalf("InjectWorktreeRules: %v", err)
	}

	// Falls back to per-worktree gitdir.
	excludePath := filepath.Join(gitDir, "info", "exclude")
	if _, err := os.ReadFile(excludePath); err != nil {
		t.Fatalf("exclude file not found (fallback): %v", err)
	}
}
