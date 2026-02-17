// Package worktree provides unified worktree management operations.
// It handles git worktree creation, removal, and discovery across the codebase.
package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager manages git worktree operations for a source repository.
type Manager struct {
	srcRepo string
}

// NewManager creates a new worktree manager for the given source repository path.
func NewManager(srcRepo string) (*Manager, error) {
	if _, err := os.Stat(srcRepo); err != nil {
		return nil, fmt.Errorf("source repo %s: %w", srcRepo, err)
	}
	return &Manager{srcRepo: srcRepo}, nil
}

// NewManagerFromWorkDir creates a new worktree manager from a workdir path.
// If workDir is a worktree, it resolves the source repository.
// If workDir is the main repo, it uses workDir as the source repository.
func NewManagerFromWorkDir(workDir string) (*Manager, error) {
	srcRepo, err := ResolveSourceRepo(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolving source repo: %w", err)
	}
	return NewManager(srcRepo)
}

// AddOptions configures how a worktree is added.
type AddOptions struct {
	// WorktreePath is the destination path for the worktree.
	WorktreePath string
	// Branch is the branch name to use. If CreateBranch is true, this branch will be created.
	Branch string
	// BaseRef is the reference to base the branch on (e.g., "origin/main").
	// Required if CreateBranch is true.
	BaseRef string
	// CreateBranch creates a new branch when adding the worktree.
	CreateBranch bool
	// DisableHooks disables git hooks during worktree operations.
	DisableHooks bool
}

// Add creates a worktree according to the provided options.
// If CreateBranch is true and the branch doesn't exist, it creates the branch from BaseRef.
// If the branch already exists, it checks out the existing branch.
func (m *Manager) Add(opts AddOptions) error {
	if opts.WorktreePath == "" {
		return fmt.Errorf("worktree path is required")
	}
	if opts.Branch == "" {
		return fmt.Errorf("branch name is required")
	}

	var gitArgs []string
	if opts.DisableHooks {
		emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
		if err != nil {
			return fmt.Errorf("create temp hooks dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(emptyHooksDir) }()
		gitArgs = append(gitArgs, "-C", m.srcRepo, "-c", "core.hooksPath="+emptyHooksDir)
	} else {
		gitArgs = append(gitArgs, "-C", m.srcRepo)
	}

	var addStderr bytes.Buffer
	branchExists := exec.Command("git", "-C", m.srcRepo, "rev-parse", "--verify", opts.Branch).Run() == nil

	if opts.CreateBranch && !branchExists {
		// Branch doesn't exist: create it when adding worktree
		if opts.BaseRef == "" {
			return fmt.Errorf("BaseRef is required when CreateBranch is true")
		}
		addCmd := exec.Command("git", append(gitArgs, "worktree", "add", "-b", opts.Branch, opts.WorktreePath, opts.BaseRef)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("git worktree add: %s", msg)
		}
		return nil
	}

	// Branch exists or CreateBranch is false: add worktree to existing branch
	addCmd := exec.Command("git", append(gitArgs, "worktree", "add", opts.WorktreePath, opts.Branch)...)
	addCmd.Stderr = &addStderr
	if err := addCmd.Run(); err != nil {
		msg := strings.TrimSpace(addStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree add: %s", msg)
	}
	return nil
}

// Remove removes a worktree at the given path.
// If idempotent is true, missing worktrees are treated as success.
func (m *Manager) Remove(worktreePath string, idempotent bool) error {
	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "remove", worktreePath, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if idempotent {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" && (strings.Contains(msg, "not found") || strings.Contains(msg, "No such file") || strings.Contains(msg, "is not a working tree")) {
				return nil
			}
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree remove: %s", msg)
	}
	return nil
}

// FindByBranch scans git worktree list output for a worktree that has the given branch checked out.
// Returns the worktree path or empty string if not found.
// If excludeSrcRepo is true, the source repository path itself is excluded from results.
func (m *Manager) FindByBranch(branchName string, excludeSrcRepo bool) string {
	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "list", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}

	// Porcelain format: blocks separated by blank lines.
	// Each block has: worktree <path>\nHEAD <sha>\nbranch refs/heads/<name>\n
	var currentPath string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			if branch == branchName && currentPath != "" {
				if excludeSrcRepo {
					// Normalize paths for comparison - resolve symlinks and make absolute
					normalizedSrc, err1 := filepath.EvalSymlinks(m.srcRepo)
					if err1 != nil {
						normalizedSrc, _ = filepath.Abs(m.srcRepo)
					}
					normalizedPath, err2 := filepath.EvalSymlinks(currentPath)
					if err2 != nil {
						normalizedPath, _ = filepath.Abs(currentPath)
					}
					// Use Clean to handle any trailing slashes or . references
					normalizedSrc = filepath.Clean(normalizedSrc)
					normalizedPath = filepath.Clean(normalizedPath)
					if normalizedPath != normalizedSrc {
						return currentPath
					}
					// Path matches srcRepo, skip it
					continue
				} else {
					return currentPath
				}
			}
		}
	}
	return ""
}

// SrcRepo returns the path to the source repository.
func (m *Manager) SrcRepo() string {
	return m.srcRepo
}
