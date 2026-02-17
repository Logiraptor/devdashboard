package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitRepo creates a temporary git repository and returns its path.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init", repo)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	// Create initial commit
	cmd = exec.Command("git", "-C", repo, "commit", "--allow-empty", "-m", "initial")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	return repo
}

func TestFindWorktreeForBranch_NotFound(t *testing.T) {
	repo := setupGitRepo(t)

	// Branch doesn't exist
	result := FindWorktreeForBranch(repo, "nonexistent-branch", false)
	if result != "" {
		t.Errorf("expected empty string for nonexistent branch, got %q", result)
	}
}

func TestFindWorktreeForBranch_FoundInMainRepo(t *testing.T) {
	repo := setupGitRepo(t)

	// Get current branch
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	branchName := strings.TrimSpace(string(out))

	// Should find the main repo when excludeSrcRepo is false
	result := FindWorktreeForBranch(repo, branchName, false)
	if result == "" {
		t.Error("expected to find main repo, got empty string")
	}
}

func TestFindWorktreeForBranch_ExcludeSrcRepo(t *testing.T) {
	repo := setupGitRepo(t)

	// Get current branch
	cmd := exec.Command("git", "-C", repo, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	branchName := strings.TrimSpace(string(out))

	// Should NOT find the main repo when excludeSrcRepo is true
	result := FindWorktreeForBranch(repo, branchName, true)
	if result != "" {
		t.Errorf("expected empty string when excluding src repo, got %q", result)
	}
}

func TestFindWorktreeForBranch_FoundInWorktree(t *testing.T) {
	repo := setupGitRepo(t)

	// Create a new branch without checking it out (so we can create a worktree for it)
	branchName := "test-branch"
	cmd := exec.Command("git", "-C", repo, "branch", branchName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git branch: %v", err)
	}

	// Create a worktree for this branch
	worktreePath := filepath.Join(t.TempDir(), "worktree")
	cmd = exec.Command("git", "-C", repo, "worktree", "add", worktreePath, branchName)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git worktree add: %v", err)
	}
	defer func() {
		exec.Command("git", "-C", repo, "worktree", "remove", worktreePath, "--force").Run()
	}()

	// Should find the worktree when excludeSrcRepo is true
	result := FindWorktreeForBranch(repo, branchName, true)
	if result == "" {
		t.Error("expected to find worktree, got empty string")
	}
	// Normalize paths for comparison (resolve symlinks)
	absWorktreePath, _ := filepath.EvalSymlinks(worktreePath)
	if absWorktreePath == "" {
		absWorktreePath, _ = filepath.Abs(worktreePath)
	}
	absResult, _ := filepath.EvalSymlinks(result)
	if absResult == "" {
		absResult, _ = filepath.Abs(result)
	}
	if absResult != absWorktreePath {
		t.Errorf("expected worktree path %q, got %q", absWorktreePath, absResult)
	}

	// Should also find it when excludeSrcRepo is false (but may return main repo or worktree)
	result2 := FindWorktreeForBranch(repo, branchName, false)
	if result2 == "" {
		t.Error("expected to find worktree or main repo, got empty string")
	}
}

func TestFindWorktreeForBranch_InvalidRepo(t *testing.T) {
	// Non-existent repo
	result := FindWorktreeForBranch("/nonexistent/repo", "branch", false)
	if result != "" {
		t.Errorf("expected empty string for invalid repo, got %q", result)
	}
}
