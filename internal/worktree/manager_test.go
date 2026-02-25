package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) string {
	dir := t.TempDir()
	// Initialize git repo
	cmd := exec.Command("git", "-C", dir, "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	// Create initial commit
	cmd = exec.Command("git", "-C", dir, "config", "user.name", "Test")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "config", "user.email", "test@example.com")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config user.email: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "checkout", "-b", "main")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git checkout -b main: %v", err)
	}
	// Create a file and commit
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "add", "test.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", "initial")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return dir
}

func TestNewManager(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m.srcRepo != srcRepo {
		t.Errorf("expected srcRepo %s, got %s", srcRepo, m.srcRepo)
	}
}

func TestNewManager_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := NewManager(dir)
	if err == nil {
		t.Fatal("expected error for non-git repo")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository' error, got %v", err)
	}
}

func TestNewManagerFromWorkDir_MainRepo(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManagerFromWorkDir(srcRepo)
	if err != nil {
		t.Fatalf("NewManagerFromWorkDir: %v", err)
	}
	if m.srcRepo != srcRepo {
		t.Errorf("expected srcRepo %s, got %s", srcRepo, m.srcRepo)
	}
}

func TestNewManagerFromWorkDir_Worktree(t *testing.T) {
	srcRepo := setupTestRepo(t)
	// Create a worktree
	wtPath := filepath.Join(t.TempDir(), "worktree")
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "add", wtPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	defer func() {
		_ = exec.Command("git", "-C", srcRepo, "worktree", "remove", wtPath, "--force").Run()
	}()

	m, err := NewManagerFromWorkDir(wtPath)
	if err != nil {
		t.Fatalf("NewManagerFromWorkDir: %v", err)
	}
	// Resolve symlinks for comparison (macOS /var -> /private/var)
	srcRepoAbs, _ := filepath.EvalSymlinks(srcRepo)
	gotAbs, _ := filepath.EvalSymlinks(m.srcRepo)
	if gotAbs != srcRepoAbs {
		t.Errorf("expected srcRepo %s (resolved: %s), got %s (resolved: %s)", srcRepo, srcRepoAbs, m.srcRepo, gotAbs)
	}
}

func TestManager_Remove(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Create a worktree
	wtPath := filepath.Join(t.TempDir(), "test-worktree")
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "add", wtPath)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree should exist: %v", err)
	}

	// Remove it
	if err := m.Remove(wtPath, false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(wtPath); err == nil {
		t.Error("worktree should not exist after removal")
	}
}

func TestManager_Remove_NonExistent(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Removing non-existent worktree should be idempotent
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	if err := m.Remove(nonExistent, true); err != nil {
		t.Errorf("Remove should be idempotent, got error: %v", err)
	}
}

func TestManager_FindByBranch(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Ensure we're on main
	cmd := exec.Command("git", "-C", srcRepo, "checkout", "main")
	if err := cmd.Run(); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	// Create a branch but don't checkout to it (so it's not checked out in main repo)
	branchName := "find-test"
	cmd = exec.Command("git", "-C", srcRepo, "branch", branchName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	// Create a worktree on that branch
	wtPath := filepath.Join(t.TempDir(), "find-worktree")
	cmd = exec.Command("git", "-C", srcRepo, "worktree", "add", wtPath, branchName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	defer func() {
		_ = m.Remove(wtPath, true)
	}()

	// Find it (excludeSrcRepo=true to find the worktree, not main repo)
	found := m.FindByBranch(branchName, true)
	// Resolve symlinks for comparison
	foundAbs, _ := filepath.EvalSymlinks(found)
	wtPathAbs, _ := filepath.EvalSymlinks(wtPath)
	if foundAbs != wtPathAbs {
		t.Errorf("expected %s (resolved: %s), got %s (resolved: %s)", wtPath, wtPathAbs, found, foundAbs)
	}

	// Non-existent branch
	notFound := m.FindByBranch("nonexistent-branch", true)
	if notFound != "" {
		t.Errorf("expected empty string, got %s", notFound)
	}
}

func TestManager_List(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Ensure we're on main before creating branches
	cmd := exec.Command("git", "-C", srcRepo, "checkout", "main")
	if err := cmd.Run(); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	// Create a couple of branches (don't checkout to them)
	branch1 := "list-test-1"
	branch2 := "list-test-2"

	cmd = exec.Command("git", "-C", srcRepo, "branch", branch1)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create branch1: %v", err)
	}

	wtPath1 := filepath.Join(t.TempDir(), "list-worktree-1")
	cmd = exec.Command("git", "-C", srcRepo, "worktree", "add", wtPath1, branch1)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create worktree1: %v", err)
	}
	defer func() {
		_ = m.Remove(wtPath1, true)
	}()

	cmd = exec.Command("git", "-C", srcRepo, "branch", branch2)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create branch2: %v", err)
	}

	wtPath2 := filepath.Join(t.TempDir(), "list-worktree-2")
	cmd = exec.Command("git", "-C", srcRepo, "worktree", "add", wtPath2, branch2)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create worktree2: %v", err)
	}
	defer func() {
		_ = m.Remove(wtPath2, true)
	}()

}

func TestManager_SrcRepo(t *testing.T) {
	srcRepo := setupTestRepo(t)
	m, err := NewManager(srcRepo)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if m.SrcRepo() != srcRepo {
		t.Errorf("expected %s, got %s", srcRepo, m.SrcRepo())
	}
}
