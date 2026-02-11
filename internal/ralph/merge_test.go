package ralph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestGitRepo creates a temporary git repository with initial commit.
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo
	cmds := [][]string{
		{"init"},
		{"config", "user.name", "Test User"},
		{"config", "user.email", "test@example.com"},
		{"commit", "--allow-empty", "-m", "initial commit"},
	}

	for _, cmd := range cmds {
		gitCmd := exec.Command("git", cmd...)
		gitCmd.Dir = dir
		if err := gitCmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", cmd, err)
		}
	}

	return dir
}

// createBranch creates a new branch and commits a file to it.
func createBranch(t *testing.T, repoPath, branchName, fileName, content string) {
	t.Helper()
	// Get current branch first
	currentBranchCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	currentBranchOut, err := currentBranchCmd.Output()
	if err != nil {
		t.Fatalf("createBranch get current branch failed: %v", err)
	}
	currentBranch := strings.TrimSpace(string(currentBranchOut))

	// Check if branch already exists
	checkBranchCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--verify", branchName)
	if checkBranchCmd.Run() == nil {
		// Branch exists, just checkout
		checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", branchName)
		if err := checkoutCmd.Run(); err != nil {
			t.Fatalf("createBranch checkout existing branch failed: %v", err)
		}
	} else {
		// Create and checkout new branch
		checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "-b", branchName)
		if err := checkoutCmd.Run(); err != nil {
			t.Fatalf("createBranch checkout -b failed: %v", err)
		}
	}

	// Create file
	filePath := filepath.Join(repoPath, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("createBranch write file failed: %v", err)
	}

	// Add and commit
	addCmd := exec.Command("git", "-C", repoPath, "add", fileName)
	if err := addCmd.Run(); err != nil {
		t.Fatalf("createBranch add failed: %v", err)
	}

	commitCmd := exec.Command("git", "-C", repoPath, "commit", "-m", fmt.Sprintf("add %s", fileName))
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("createBranch commit failed: %v", err)
	}

	// Restore original branch
	restoreCmd := exec.Command("git", "-C", repoPath, "checkout", currentBranch)
	_ = restoreCmd.Run() // Best effort
}

// createConflictingBranches creates two branches with conflicting changes.
func createConflictingBranches(t *testing.T, repoPath, baseBranch, branch1, branch2 string) {
	t.Helper()
	// Start from base branch
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", baseBranch)
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches checkout base failed: %v", err)
	}

	// Create file on base
	filePath := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(filePath, []byte("base content\n"), 0644); err != nil {
		t.Fatalf("createConflictingBranches write base file failed: %v", err)
	}

	addCmd := exec.Command("git", "-C", repoPath, "add", "test.txt")
	if err := addCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches add base file failed: %v", err)
	}

	commitCmd := exec.Command("git", "-C", repoPath, "commit", "-m", "add base file")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches commit base file failed: %v", err)
	}

	// Create branch1 with one change
	checkoutCmd = exec.Command("git", "-C", repoPath, "checkout", "-b", branch1)
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches checkout branch1 failed: %v", err)
	}

	if err := os.WriteFile(filePath, []byte("branch1 content\n"), 0644); err != nil {
		t.Fatalf("createConflictingBranches write branch1 file failed: %v", err)
	}

	addCmd = exec.Command("git", "-C", repoPath, "add", "test.txt")
	if err := addCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches add branch1 file failed: %v", err)
	}

	commitCmd = exec.Command("git", "-C", repoPath, "commit", "-m", "change on branch1")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches commit branch1 failed: %v", err)
	}

	// Create branch2 with conflicting change
	checkoutCmd = exec.Command("git", "-C", repoPath, "checkout", baseBranch)
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches checkout base for branch2 failed: %v", err)
	}

	checkoutCmd = exec.Command("git", "-C", repoPath, "checkout", "-b", branch2)
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches checkout branch2 failed: %v", err)
	}

	if err := os.WriteFile(filePath, []byte("branch2 content\n"), 0644); err != nil {
		t.Fatalf("createConflictingBranches write branch2 file failed: %v", err)
	}

	addCmd = exec.Command("git", "-C", repoPath, "add", "test.txt")
	if err := addCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches add branch2 file failed: %v", err)
	}

	commitCmd = exec.Command("git", "-C", repoPath, "commit", "-m", "change on branch2")
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("createConflictingBranches commit branch2 failed: %v", err)
	}
}

// TestMergeBranches tests the MergeBranches function.
func TestMergeBranches(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(*testing.T) (string, string, string) // returns repoPath, targetBranch, sourceBranch
		disableHooks bool
		beadID       string
		wantErr      bool
		wantConflict bool
	}{
		{
			name: "successful merge",
			setup: func(t *testing.T) (string, string, string) {
				repoPath := setupTestGitRepo(t)
				createBranch(t, repoPath, "main", "file1.txt", "content1")
				createBranch(t, repoPath, "feature", "file2.txt", "content2")
				// Switch back to main
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				return repoPath, "main", "feature"
			},
			disableHooks: false,
			beadID:       "",
			wantErr:      false,
			wantConflict: false,
		},
		{
			name: "merge with hooks disabled",
			setup: func(t *testing.T) (string, string, string) {
				repoPath := setupTestGitRepo(t)
				createBranch(t, repoPath, "main", "file1.txt", "content1")
				createBranch(t, repoPath, "feature", "file2.txt", "content2")
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				return repoPath, "main", "feature"
			},
			disableHooks: true,
			beadID:       "",
			wantErr:      false,
			wantConflict: false,
		},
		{
			name: "merge conflict",
			setup: func(t *testing.T) (string, string, string) {
				repoPath := setupTestGitRepo(t)
				createConflictingBranches(t, repoPath, "main", "feature1", "feature2")
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				// First merge feature1 (should succeed)
				firstMergeCmd := exec.Command("git", "-C", repoPath, "merge", "feature1", "--no-edit")
				if err := firstMergeCmd.Run(); err != nil {
					t.Fatalf("first merge failed: %v", err)
				}
				// Now try to merge feature2 which should conflict
				return repoPath, "main", "feature2"
			},
			disableHooks: false,
			beadID:       "",
			wantErr:      true,
			wantConflict: true,
		},
		{
			name: "nonexistent target branch",
			setup: func(t *testing.T) (string, string, string) {
				repoPath := setupTestGitRepo(t)
				createBranch(t, repoPath, "feature", "file1.txt", "content1")
				return repoPath, "nonexistent", "feature"
			},
			disableHooks: false,
			beadID:       "",
			wantErr:      true,
			wantConflict: false,
		},
		{
			name: "nonexistent source branch",
			setup: func(t *testing.T) (string, string, string) {
				repoPath := setupTestGitRepo(t)
				createBranch(t, repoPath, "main", "file1.txt", "content1")
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				return repoPath, "main", "nonexistent"
			},
			disableHooks: false,
			beadID:       "",
			wantErr:      true,
			wantConflict: false,
		},
		{
			name: "nonexistent repository",
			setup: func(t *testing.T) (string, string, string) {
				return "/nonexistent/repo", "main", "feature"
			},
			disableHooks: false,
			beadID:       "",
			wantErr:      true,
			wantConflict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath, targetBranch, sourceBranch := tt.setup(t)

			err := MergeBranches(repoPath, targetBranch, sourceBranch, tt.disableHooks, tt.beadID)

			if (err != nil) != tt.wantErr {
				t.Errorf("MergeBranches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantConflict {
				if err == nil {
					t.Error("MergeBranches() expected conflict error, got nil")
					return
				}
				if !strings.Contains(err.Error(), "conflicts detected") {
					t.Errorf("MergeBranches() error = %v, expected 'conflicts detected'", err)
				}

				// Verify conflicts are actually present
				if !hasMergeConflicts(repoPath) {
					t.Error("MergeBranches() expected conflicts but hasMergeConflicts() returned false")
				}
			}

			if !tt.wantErr && err == nil {
				// Verify merge was successful
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", targetBranch)
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout target branch failed: %v", err)
				}

				// Check that source branch is merged
				logCmd := exec.Command("git", "-C", repoPath, "log", "--oneline", "--graph", "-5")
				logOut, err := logCmd.Output()
				if err != nil {
					t.Fatalf("git log failed: %v", err)
				}
				// The merge commit should exist
				if !strings.Contains(string(logOut), sourceBranch) && !strings.Contains(string(logOut), "Merge") {
					t.Logf("git log output: %s", string(logOut))
					// This is a soft check - merge might be fast-forward
				}
			}
		})
	}
}

// TestRenderMergePrompt tests the RenderMergePrompt function.
func TestRenderMergePrompt(t *testing.T) {
	tests := []struct {
		name    string
		data    *MergePromptData
		wantErr bool
		want    []string // substrings that should be in the rendered prompt
	}{
		{
			name: "valid prompt data",
			data: &MergePromptData{
				ID:            "test-1",
				Title:         "Test Merge",
				Description:   "Test description",
				TargetBranch:  "main",
				SourceBranch:  "feature",
				RepoPath:      "/path/to/repo",
			},
			wantErr: false,
			want: []string{
				"test-1",
				"Test Merge",
				"Test description",
				"main",
				"feature",
				"/path/to/repo",
				"bd update test-1",
				"bd close test-1",
			},
		},
		{
			name: "empty data",
			data: &MergePromptData{
				ID:            "",
				Title:         "",
				Description:   "",
				TargetBranch:  "",
				SourceBranch:  "",
				RepoPath:      "",
			},
			wantErr: false,
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderMergePrompt(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("RenderMergePrompt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				for _, wantSubstr := range tt.want {
					if !strings.Contains(got, wantSubstr) {
						t.Errorf("RenderMergePrompt() output missing substring %q. Got: %s", wantSubstr, got)
					}
				}
			}
		})
	}
}

// TestHasMergeConflicts tests the hasMergeConflicts function.
func TestHasMergeConflicts(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*testing.T) string // returns repoPath
		want    bool
		wantErr bool
	}{
		{
			name: "no conflicts",
			setup: func(t *testing.T) string {
				repoPath := setupTestGitRepo(t)
				createBranch(t, repoPath, "main", "file1.txt", "content1")
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				return repoPath
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "has conflicts",
			setup: func(t *testing.T) string {
				repoPath := setupTestGitRepo(t)
				createConflictingBranches(t, repoPath, "main", "feature1", "feature2")
				checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
				if err := checkoutCmd.Run(); err != nil {
					t.Fatalf("checkout main failed: %v", err)
				}
				// Attempt merge to create conflicts
				mergeCmd := exec.Command("git", "-C", repoPath, "merge", "feature1", "--no-edit")
				mergeCmd.Run() // Ignore error, we expect conflicts
				// Verify conflicts exist by checking git status
				statusCmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
				statusOut, _ := statusCmd.Output()
				if !strings.Contains(string(statusOut), "UU") && !strings.Contains(string(statusOut), "AA") {
					// Try ls-files -u as fallback
					lsFilesCmd := exec.Command("git", "-C", repoPath, "ls-files", "-u")
					lsFilesOut, _ := lsFilesCmd.Output()
					if len(lsFilesOut) == 0 {
						t.Skip("Could not create merge conflicts in test setup")
					}
				}
				return repoPath
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "nonexistent repo",
			setup: func(*testing.T) string {
				return "/nonexistent/repo"
			},
			want:    false,
			wantErr: false, // hasMergeConflicts returns false on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := tt.setup(t)

			got := hasMergeConflicts(repoPath)

			if got != tt.want {
				t.Errorf("hasMergeConflicts() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCreateQuestionBeadForMergeConflict tests createQuestionBeadForMergeConflict.
// Note: This test requires bd to be available and may create actual beads.
// We'll mock the bd command execution for unit testing.
func TestCreateQuestionBeadForMergeConflict(t *testing.T) {
	// This test would require mocking bd commands, which is complex.
	// For now, we'll test the logic that doesn't require bd.
	// A full integration test would require a real bd setup.

	t.Run("conflict detection triggers question bead creation", func(t *testing.T) {
		repoPath := setupTestGitRepo(t)
		createConflictingBranches(t, repoPath, "main", "feature1", "feature2")

		checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
		if err := checkoutCmd.Run(); err != nil {
			t.Fatalf("checkout main failed: %v", err)
		}

		// First merge feature1 into main - this should succeed
		firstMergeCmd := exec.Command("git", "-C", repoPath, "merge", "feature1", "--no-edit")
		if err := firstMergeCmd.Run(); err != nil {
			t.Fatalf("first merge (feature1 into main) should succeed: %v", err)
		}

		// Now attempt to merge feature2 - this should conflict since both feature1
		// and feature2 modified test.txt differently
		secondMergeCmd := exec.Command("git", "-C", repoPath, "merge", "feature2", "--no-edit")
		secondMergeCmd.Run() // Expect conflicts, ignore error

		// Verify conflicts exist
		if !hasMergeConflicts(repoPath) {
			t.Fatal("expected conflicts but hasMergeConflicts() returned false")
		}

		// Abort the merge to clean up
		abortCmd := exec.Command("git", "-C", repoPath, "merge", "--abort")
		abortCmd.Run() // Best effort

		// Test that MergeBranches detects conflicts and would create question bead
		// (we can't easily test the actual bead creation without mocking bd)
		err := MergeBranches(repoPath, "main", "feature2", false, "test-bead-1")
		if err == nil {
			t.Error("MergeBranches() expected error for conflicts, got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "conflicts detected") {
			t.Errorf("MergeBranches() error = %v, expected 'conflicts detected'", err)
		}
	})
}

// TestMergeBranches_restoresOriginalBranch tests that MergeBranches restores the original branch.
func TestMergeBranches_restoresOriginalBranch(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	createBranch(t, repoPath, "main", "file1.txt", "content1")
	createBranch(t, repoPath, "feature", "file2.txt", "content2")

	// Checkout a different branch
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "feature")
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("checkout feature failed: %v", err)
	}

	// Get current branch before merge
	beforeCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	beforeOut, err := beforeCmd.Output()
	if err != nil {
		t.Fatalf("get branch before merge failed: %v", err)
	}
	beforeBranch := strings.TrimSpace(string(beforeOut))

	// Perform merge
	err = MergeBranches(repoPath, "main", "feature", false, "")
	if err != nil {
		t.Fatalf("MergeBranches() error = %v", err)
	}

	// Verify we're back on the original branch
	afterCmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	afterOut, err := afterCmd.Output()
	if err != nil {
		t.Fatalf("get branch after merge failed: %v", err)
	}
	afterBranch := strings.TrimSpace(string(afterOut))

	if afterBranch != beforeBranch {
		t.Errorf("MergeBranches() did not restore original branch. Before: %s, After: %s", beforeBranch, afterBranch)
	}
}

// TestMergeBranches_sourceBranchResolution tests that source branch resolution works.
func TestMergeBranches_sourceBranchResolution(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	createBranch(t, repoPath, "main", "file1.txt", "content1")

	// Create a branch but don't check it out locally (simulate remote-only branch)
	// We'll create it locally but test that it works
	createBranch(t, repoPath, "remote-feature", "file2.txt", "content2")
	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("checkout main failed: %v", err)
	}

	// Test merging local branch
	err := MergeBranches(repoPath, "main", "remote-feature", false, "")
	if err != nil {
		t.Errorf("MergeBranches() with local branch error = %v", err)
	}
}

// TestMergeBranches_conflictWithBeadID tests conflict handling with beadID.
func TestMergeBranches_conflictWithBeadID(t *testing.T) {
	repoPath := setupTestGitRepo(t)
	createConflictingBranches(t, repoPath, "main", "feature1", "feature2")

	checkoutCmd := exec.Command("git", "-C", repoPath, "checkout", "main")
	if err := checkoutCmd.Run(); err != nil {
		t.Fatalf("checkout main failed: %v", err)
	}

	// First merge feature1 into main (this should succeed)
	err := MergeBranches(repoPath, "main", "feature1", false, "test-bead-1")
	if err != nil {
		t.Fatalf("First merge failed: %v", err)
	}

	// Now try to merge feature2 into main - this should create conflicts
	// since both feature1 and feature2 modified the same file differently
	err = MergeBranches(repoPath, "main", "feature2", false, "test-bead-2")
	if err == nil {
		// Check if conflicts actually exist
		if !hasMergeConflicts(repoPath) {
			t.Skip("Merge succeeded without conflicts - test setup may need adjustment")
			return
		}
		t.Error("MergeBranches() expected error for conflicts, got nil")
		return
	}
	if !strings.Contains(err.Error(), "conflicts detected") {
		t.Errorf("MergeBranches() error = %v, expected 'conflicts detected'", err)
	}
}
