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

// TestManager_Create_Ralph is commented out - this functionality has been moved to
// internal/ralph/worktree.go. The worktree package now provides low-level worktree
// operations (Add, Remove, FindByBranch) that are used by higher-level packages.
// func TestManager_Create_Ralph(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
//
// 	opts := Options{
// 		UseCase:    "ralph",
// 		BaseWorkDir: srcRepo,
// 		BeadID:     "test-bead-123",
// 	}
//
// 	wtPath, branchName, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create: %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath, true)
// 	}
//
// 	expectedBranch := "ralph/test-bead-123"
// 	if branchName != expectedBranch {
// 		t.Errorf("expected branch %s, got %s", expectedBranch, branchName)
// 	}
//
// 	expectedPath := filepath.Join(os.TempDir(), "ralph-test-bead-123")
// 	// Resolve symlinks for comparison (macOS /var -> /private/var)
// 	expectedPathAbs, _ := filepath.EvalSymlinks(expectedPath)
// 	wtPathAbs, _ := filepath.EvalSymlinks(wtPath)
// 	if wtPathAbs != expectedPathAbs {
// 		t.Errorf("expected path %s (resolved: %s), got %s (resolved: %s)", expectedPath, expectedPathAbs, wtPath, wtPathAbs)
// 	}
//
// 	// Verify worktree exists
// 	if _, err := os.Stat(wtPath); err != nil {
// 		t.Errorf("worktree path does not exist: %v", err)
// 	}
// }

// All TestManager_Create_* tests are commented out - this functionality has been moved to
// internal/ralph/worktree.go and internal/project/project.go. The worktree package now provides
// low-level worktree operations (Add, Remove, FindByBranch) that are used by higher-level packages.

// // func TestManager_Create_Ralph_ReusesExistingBranch(t *testing.T) {
// // 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	beadID := "test-bead-456"
// 	opts := Options{
// 		UseCase:    "ralph",
// 		BaseWorkDir: srcRepo,
// 		BeadID:     beadID,
// 	}
// 
// 	// Create first worktree
// 	wtPath1, branchName1, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create (first): %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath1)
// 	}()
// 
// 	// Remove the first worktree but keep the branch
// 	if err := m.Remove(wtPath1, false); err != nil {
// 		t.Fatalf("Remove first worktree: %v", err)
// 	}
// 
// 	// Create second worktree with same bead ID (should reuse branch but create new worktree)
// 	wtPath2, branchName2, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create (second): %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath2)
// 	}()
// 
// 	if branchName1 != branchName2 {
// 		t.Errorf("expected same branch name, got %s and %s", branchName1, branchName2)
// 	}
// 	// Should reuse the same path since it's based on beadID
// 	wtPath1Abs, _ := filepath.EvalSymlinks(wtPath1)
// 	wtPath2Abs, _ := filepath.EvalSymlinks(wtPath2)
// 	if wtPath1Abs != wtPath2Abs {
// 		t.Errorf("expected same worktree path (reused), got %s and %s", wtPath1Abs, wtPath2Abs)
// 	}
// }
// 
// func TestManager_Create_Project(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	// Ensure we're on main
// 	cmd := exec.Command("git", "-C", srcRepo, "checkout", "main")
// 	if err := cmd.Run(); err != nil {
// 		t.Fatalf("checkout main: %v", err)
// 	}
// 
// 	// Create a branch for the PR (don't checkout to it)
// 	branchName := "feat-test"
// 	cmd = exec.Command("git", "-C", srcRepo, "branch", branchName)
// 	if err := cmd.Run(); err != nil {
// 		t.Fatalf("create branch: %v", err)
// 	}
// 
// 	projectDir := t.TempDir()
// 	opts := Options{
// 		UseCase:   "project",
// 		ProjectDir: projectDir,
// 		RepoName:  "my-repo",
// 		PRNumber:  42,
// 		BranchName: branchName,
// 	}
// 
// 	wtPath, gotBranch, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create: %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath, true)
// 	}()
// 
// 	if gotBranch != branchName {
// 		t.Errorf("expected branch %s, got %s", branchName, gotBranch)
// 	}
// 
// 	expectedPath := filepath.Join(projectDir, "my-repo-pr-42")
// 	// Resolve symlinks for comparison (macOS /var -> /private/var)
// 	projectDirAbs, _ := filepath.EvalSymlinks(projectDir)
// 	expectedPathAbs := filepath.Join(projectDirAbs, "my-repo-pr-42")
// 	wtPathAbs, _ := filepath.EvalSymlinks(wtPath)
// 	if wtPathAbs != expectedPathAbs {
// 		t.Errorf("expected path %s (resolved: %s), got %s (resolved: %s)", expectedPath, expectedPathAbs, wtPath, wtPathAbs)
// 	}
// 
// 	// Verify worktree exists
// 	if _, err := os.Stat(wtPath); err != nil {
// 		t.Errorf("worktree path does not exist: %v", err)
// 	}
// }
// 
// func TestManager_Create_Project_ReusesExisting(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	// Create a branch for the PR
// 	branchName := "feat-reuse"
// 	cmd := exec.Command("git", "-C", srcRepo, "checkout", "-b", branchName)
// 	if err := cmd.Run(); err != nil {
// 		t.Fatalf("create branch: %v", err)
// 	}
// 
// 	projectDir := t.TempDir()
// 	opts := Options{
// 		UseCase:   "project",
// 		ProjectDir: projectDir,
// 		RepoName:  "my-repo",
// 		PRNumber:  99,
// 		BranchName: branchName,
// 	}
// 
// 	// Create first worktree
// 	wtPath1, _, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create (first): %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath1)
// 	}()
// 
// 	// Create second worktree with same options (should reuse)
// 	wtPath2, _, err := m.Create(opts)
// 	if err != nil {
// 		t.Fatalf("Create (second): %v", err)
// 	}
// 	defer func() {
// 		_ = m.Remove(wtPath2)
// 	}()
// 
// 	if wtPath1 != wtPath2 {
// 		t.Errorf("expected same worktree path, got %s and %s", wtPath1, wtPath2)
// 	}
// }
// 
// func TestManager_Create_InvalidUseCase(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	opts := Options{
// 		UseCase: "invalid",
// 	}
// 
// 	_, _, err = m.Create(opts)
// 	if err == nil {
// 		t.Fatal("expected error for invalid use case")
// 	}
// 	if !strings.Contains(err.Error(), "unknown use case") {
// 		t.Errorf("expected 'unknown use case' error, got %v", err)
// 	}
// }
// 
// func TestManager_Create_Ralph_MissingBeadID(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	opts := Options{
// 		UseCase:    "ralph",
// 		BaseWorkDir: srcRepo,
// 		// Missing BeadID
// 	}
// 
// 	_, _, err = m.Create(opts)
// 	if err == nil {
// 		t.Fatal("expected error for missing BeadID")
// 	}
// 	if !strings.Contains(err.Error(), "BeadID is required") {
// 		t.Errorf("expected 'BeadID is required' error, got %v", err)
// 	}
// }
// 
// func TestManager_Create_Project_MissingFields(t *testing.T) {
// 	srcRepo := setupTestRepo(t)
// 	m, err := NewManager(srcRepo)
// 	if err != nil {
// 		t.Fatalf("NewManager: %v", err)
// 	}
// 
// 	tests := []struct {
// 		name string
// 		opts Options
// 		want string
// 	}{
// 		{
// 			name: "missing ProjectDir",
// 			opts: Options{
// 				UseCase:   "project",
// 				RepoName:  "my-repo",
// 				PRNumber:  42,
// 				BranchName: "feat-branch",
// 			},
// 			want: "ProjectDir is required",
// 		},
// 		{
// 			name: "missing RepoName",
// 			opts: Options{
// 				UseCase:   "project",
// 				ProjectDir: t.TempDir(),
// 				PRNumber:  42,
// 				BranchName: "feat-branch",
// 			},
// 			want: "RepoName is required",
// 		},
// 		{
// 			name: "missing PRNumber",
// 			opts: Options{
// 				UseCase:   "project",
// 				ProjectDir: t.TempDir(),
// 				RepoName:  "my-repo",
// 				BranchName: "feat-branch",
// 			},
// 			want: "PRNumber is required",
// 		},
// 		{
// 			name: "missing BranchName",
// 			opts: Options{
// 				UseCase:   "project",
// 				ProjectDir: t.TempDir(),
// 				RepoName:  "my-repo",
// 				PRNumber:  42,
// 			},
// 			want: "BranchName is required",
// 		},
// 	}
// 
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			_, _, err := m.Create(tt.opts)
// 			if err == nil {
// 				t.Fatal("expected error")
// 			}
// 			if !strings.Contains(err.Error(), tt.want) {
// 				t.Errorf("expected error containing %q, got %v", tt.want, err)
// 			}
// 		})
// 	}
// }

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

	// List worktrees test commented out - Manager.List() doesn't exist
	// The worktree package provides List() as a standalone function, not a Manager method
	// If this functionality is needed, use worktree.List() instead
	// worktrees, err := m.List()
	// if err != nil {
	// 	t.Fatalf("List: %v", err)
	// }
	// ... rest of test commented out
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
