package ralph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeManager manages temporary git worktrees for concurrent agent execution.
type WorktreeManager struct {
	// baseWorkDir is the original workdir (may be a worktree itself)
	baseWorkDir string
	// srcRepo is the path to the main git repository
	srcRepo string
	// branch is the branch name to use for worktrees
	branch string
}

// NewWorktreeManager creates a new worktree manager for the given workdir.
// It resolves the source repository and current branch.
func NewWorktreeManager(workDir string) (*WorktreeManager, error) {
	// Resolve the source repository.
	// If workDir is a worktree, .git is a file pointing to the main repo.
	// If workDir is the main repo, .git is a directory.
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	var srcRepo string
	if info.IsDir() {
		// This is the main repository
		srcRepo = workDir
	} else {
		// This is a worktree; read the .git file to find the main repo
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return nil, fmt.Errorf("reading .git file: %w", err)
		}
		// Format: "gitdir: /path/to/main/repo/.git/worktrees/worktree-name"
		gitdir := strings.TrimSpace(string(data))
		if !strings.HasPrefix(gitdir, "gitdir: ") {
			return nil, fmt.Errorf("invalid .git file format: %q", gitdir)
		}
		gitdir = strings.TrimPrefix(gitdir, "gitdir: ")
		// Extract the main repo path: remove "/.git/worktrees/..." suffix
		parts := strings.Split(gitdir, "/.git/worktrees/")
		if len(parts) == 0 {
			return nil, fmt.Errorf("cannot parse gitdir: %q", gitdir)
		}
		srcRepo = parts[0]
	}

	// Get the current branch name
	branch, err := getCurrentBranch(workDir)
	if err != nil {
		return nil, fmt.Errorf("getting current branch: %w", err)
	}

	return &WorktreeManager{
		baseWorkDir: workDir,
		srcRepo:     srcRepo,
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
	return w.srcRepo
}

// Branch returns the branch name used for worktrees.
func (w *WorktreeManager) Branch() string {
	return w.branch
}

// CreateWorktree creates a temporary worktree for the given bead ID.
// Returns the path to the worktree directory and the branch name used.
func (w *WorktreeManager) CreateWorktree(beadID string) (worktreePath string, branchName string, err error) {
	// Create worktree in /tmp/ralph-<bead-id>
	worktreePath = filepath.Join(os.TempDir(), fmt.Sprintf("ralph-%s", beadID))
	// Create a unique branch name for this worktree
	branchName = fmt.Sprintf("ralph/%s", beadID)

	// Empty dir for core.hooksPath to disable hooks
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return "", "", fmt.Errorf("create temp hooks dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(emptyHooksDir) }()
	gitNoHooks := []string{"-C", w.srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Check if branch already exists
	var addStderr strings.Builder
	if err := exec.Command("git", "-C", w.srcRepo, "rev-parse", "--verify", branchName).Run(); err != nil {
		// Branch doesn't exist: create it when adding worktree
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", branchName, worktreePath, w.branch)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree for %s: %s: %w", beadID, msg, err)
			}
			return "", "", fmt.Errorf("creating worktree for %s: %w", beadID, err)
		}
	} else {
		// Branch exists: add worktree to existing branch
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", worktreePath, branchName)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree for %s: %s: %w", beadID, msg, err)
			}
			return "", "", fmt.Errorf("creating worktree for %s: %w", beadID, err)
		}
	}

	return worktreePath, branchName, nil
}

// RemoveWorktree removes a worktree created by CreateWorktree.
// The associated branch (ralph/<beadID>) is preserved, as it may have been
// pushed to remote or referenced elsewhere. To clean up branches, use
// 'git branch -D <branchName>' separately.
func (w *WorktreeManager) RemoveWorktree(worktreePath string) error {
	// Remove the worktree
	cmd := exec.Command("git", "-C", w.srcRepo, "worktree", "remove", worktreePath, "--force")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		// If worktree doesn't exist, that's okay (idempotent)
		if msg != "" && (strings.Contains(msg, "not found") || strings.Contains(msg, "No such file")) {
			return nil
		}
		if msg != "" {
			return fmt.Errorf("removing worktree %s: %s: %w", worktreePath, msg, err)
		}
		return fmt.Errorf("removing worktree %s: %w", worktreePath, err)
	}

	// Note: Branch is intentionally preserved after worktree removal.
	// The branch may have been pushed to remote or referenced elsewhere.
	// To delete the branch, use: git branch -D <branchName>

	return nil
}
