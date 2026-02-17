package worktree

import (
	"os"
	"path/filepath"
	"testing"
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

func TestResolveCommonDir_RegularRepo(t *testing.T) {
	// Test with a regular repo (where .git is a directory, not a file).
	// The common dir IS the .git/ directory itself.
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	commonDir, err := ResolveCommonDir(repo)
	if err != nil {
		t.Fatalf("ResolveCommonDir: %v", err)
	}

	if commonDir != gitDir {
		t.Errorf("expected common dir %q, got %q", gitDir, commonDir)
	}
}

func TestResolveCommonDir_Worktree(t *testing.T) {
	wt, commonDir := setupWorktreeDir(t)

	resolved, err := ResolveCommonDir(wt)
	if err != nil {
		t.Fatalf("ResolveCommonDir: %v", err)
	}

	if resolved != commonDir {
		t.Errorf("expected common dir %q, got %q", commonDir, resolved)
	}
}

func TestResolveCommonDir_WorktreeRelativeGitDir(t *testing.T) {
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

	resolved, err := ResolveCommonDir(wt)
	if err != nil {
		t.Fatalf("ResolveCommonDir: %v", err)
	}

	if resolved != commonDir {
		t.Errorf("expected common dir %q, got %q", commonDir, resolved)
	}
}

func TestResolveCommonDir_NoCommonDirFile(t *testing.T) {
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

	resolved, err := ResolveCommonDir(wt)
	if err != nil {
		t.Fatalf("ResolveCommonDir: %v", err)
	}

	// Falls back to per-worktree gitdir.
	if resolved != gitDir {
		t.Errorf("expected fallback to gitdir %q, got %q", gitDir, resolved)
	}
}

func TestResolveCommonDir_NotAGitRepo(t *testing.T) {
	base := t.TempDir()
	notRepo := filepath.Join(base, "not-a-repo")
	if err := os.MkdirAll(notRepo, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveCommonDir(notRepo)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestResolveCommonDir_InvalidGitFileFormat(t *testing.T) {
	base := t.TempDir()
	wt := filepath.Join(base, "worktree")
	if err := os.MkdirAll(wt, 0755); err != nil {
		t.Fatal(err)
	}
	// Invalid .git file format
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("invalid format\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveCommonDir(wt)
	if err == nil {
		t.Error("expected error for invalid .git file format")
	}
}
