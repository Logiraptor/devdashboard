package ralph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"devdeploy/internal/worktree"
)

// WorktreeManager manages temporary git worktrees for concurrent agent execution.
// It wraps worktree.Manager with ralph-specific logic: temporary paths (/tmp/ralph-*),
// branch preservation, and MergeRepo functionality.
type WorktreeManager struct {
	// baseWorkDir is the original workdir (may be a worktree itself)
	baseWorkDir string
	// mgr is the unified worktree manager
	mgr *worktree.Manager
	// branch is the branch name to use for worktrees
	branch string
}

// NewWorktreeManager creates a new worktree manager for the given workdir.
// It resolves the source repository and current branch.
func NewWorktreeManager(workDir string) (*WorktreeManager, error) {
	// Resolve the source repository
	srcRepo, err := worktree.ResolveSourceRepo(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving source repo: %w", err)
	}

	// Create unified worktree manager
	mgr, err := worktree.NewManager(srcRepo)
	if err != nil {
		return nil, fmt.Errorf("creating worktree manager: %w", err)
	}

	// Get the current branch name
	branch, err := getCurrentBranch(workDir)
	if err != nil {
		return nil, fmt.Errorf("getting current branch: %w", err)
	}

	return &WorktreeManager{
		baseWorkDir: workDir,
		mgr:         mgr,
		branch:      branch,
	}, nil
}

// getCurrentBranch returns the current branch name for the given workdir.
func getCurrentBranch(workDir string) (string, error) {
	cmd := exec.Command("git", "-C", workDir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SrcRepo returns the path to the main git repository.
func (w *WorktreeManager) SrcRepo() string {
	return w.mgr.SrcRepo()
}

// Branch returns the branch name used for worktrees.
func (w *WorktreeManager) Branch() string {
	return w.branch
}

// CreateWorktree creates a temporary worktree for the given bead ID.
// Returns the path to the worktree directory and the branch name used.
// Uses ralph-specific temporary paths: /tmp/ralph-<bead-id>
func (w *WorktreeManager) CreateWorktree(beadID string) (worktreePath string, branchName string, err error) {
	// Create worktree in /tmp/ralph-<bead-id> (ralph-specific)
	worktreePath = filepath.Join(os.TempDir(), fmt.Sprintf("ralph-%s", beadID))
	// Create a unique branch name for this worktree
	branchName = fmt.Sprintf("ralph/%s", beadID)

	// Use unified worktree manager to create the worktree
	err = w.mgr.Add(worktree.AddOptions{
		WorktreePath: worktreePath,
		Branch:       branchName,
		BaseRef:      w.branch,
		CreateBranch: true,
		DisableHooks: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("creating worktree for %s: %w", beadID, err)
	}

	return worktreePath, branchName, nil
}

// MergeRepo returns the repository path to use for merging.
// If baseWorkDir is already on the target branch, use it.
// Otherwise, find the worktree that has the target branch.
// Falls back to srcRepo if no worktree is found (though this may fail if branch is checked out elsewhere).
func (w *WorktreeManager) MergeRepo(targetBranch string) string {
	// Check if baseWorkDir is on the target branch
	if currentBranch, err := getCurrentBranch(w.baseWorkDir); err == nil && currentBranch == targetBranch {
		return w.baseWorkDir
	}
	// Find worktree with target branch
	if wtPath := worktree.FindWorktreeForBranch(w.mgr.SrcRepo(), targetBranch, false); wtPath != "" {
		return wtPath
	}
	// Fallback to srcRepo (may fail if branch is checked out elsewhere)
	return w.mgr.SrcRepo()
}

// RemoveWorktree removes a worktree created by CreateWorktree.
// The associated branch (ralph/<beadID>) is preserved, as it may have been
// pushed to remote or referenced elsewhere. To clean up branches, use
// 'git branch -D <branchName>' separately.
// Uses idempotent=true so missing worktrees are treated as success.
func (w *WorktreeManager) RemoveWorktree(worktreePath string) error {
	// Use unified worktree manager with idempotent=true
	if err := w.mgr.Remove(worktreePath, true); err != nil {
		return err
	}

	// Note: Branch is intentionally preserved after worktree removal.
	// The branch may have been pushed to remote or referenced elsewhere.
	// To delete the branch, use: git branch -D <branchName>

	return nil
}
