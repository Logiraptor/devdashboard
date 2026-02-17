// Package worktree provides unified git worktree management functionality.
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager manages git worktrees for a source repository.
type Manager struct {
	srcRepo string
}

// NewManager creates a new worktree manager for the given source repository.
func NewManager(srcRepo string) (*Manager, error) {
	// Verify srcRepo exists and is a git repository
	gitPath := filepath.Join(srcRepo, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	if !info.IsDir() {
		// This is a worktree, not the main repo
		return nil, fmt.Errorf("srcRepo must be the main repository, not a worktree")
	}
	return &Manager{srcRepo: srcRepo}, nil
}

// Create creates a new worktree at the given path for the specified branch.
// If the branch doesn't exist, it will be created from baseBranch.
// Returns the worktree path and branch name used.
func (m *Manager) Create(path, branchName, baseBranch string) (string, string, error) {
	// Empty dir for core.hooksPath to disable hooks
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return "", "", fmt.Errorf("create temp hooks dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(emptyHooksDir) }()
	gitNoHooks := []string{"-C", m.srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Check if branch already exists
	var addStderr strings.Builder
	if err := exec.Command("git", "-C", m.srcRepo, "rev-parse", "--verify", branchName).Run(); err != nil {
		// Branch doesn't exist: create it when adding worktree
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", branchName, path, baseBranch)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree: %s: %w", msg, err)
			}
			return "", "", fmt.Errorf("creating worktree: %w", err)
		}
	} else {
		// Branch exists: add worktree to existing branch
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", path, branchName)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree: %s: %w", msg, err)
			}
			return "", "", fmt.Errorf("creating worktree: %w", err)
		}
	}

	return path, branchName, nil
}

// Remove removes a worktree at the given path.
// If idempotent is true, missing worktrees are treated as success.
func (m *Manager) Remove(path string, idempotent bool) error {
	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "remove", path, "--force")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if idempotent {
			msg := strings.TrimSpace(stderr.String())
			if msg != "" && (strings.Contains(msg, "not found") || strings.Contains(msg, "No such file")) {
				return nil
			}
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("removing worktree %s: %s: %w", path, msg, err)
		}
		return fmt.Errorf("removing worktree %s: %w", path, err)
	}
	return nil
}

// FindByBranch finds the worktree that has the given branch checked out.
// Returns the worktree path, or empty string if not found.
func (m *Manager) FindByBranch(branchName string) string {
	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "list", "--porcelain")
	var out strings.Builder
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
			if branch == branchName && currentPath != "" && currentPath != m.srcRepo {
				return currentPath
			}
		}
	}
	return ""
}

// SrcRepo returns the path to the source repository.
func (m *Manager) SrcRepo() string {
	return m.srcRepo
}

// ResolveSrcRepo resolves the source repository path from a workdir.
// If workDir is a worktree, it reads the .git file to find the main repo.
// If workDir is the main repo, it returns workDir.
func ResolveSrcRepo(workDir string) (string, error) {
	gitPath := filepath.Join(workDir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	if info.IsDir() {
		// This is the main repository
		return workDir, nil
	}

	// This is a worktree; read the .git file to find the main repo
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("reading .git file: %w", err)
	}
	// Format: "gitdir: /path/to/main/repo/.git/worktrees/worktree-name"
	gitdir := strings.TrimSpace(string(data))
	if !strings.HasPrefix(gitdir, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format: %q", gitdir)
	}
	gitdir = strings.TrimPrefix(gitdir, "gitdir: ")
	// Extract the main repo path: remove "/.git/worktrees/..." suffix
	parts := strings.Split(gitdir, "/.git/worktrees/")
	if len(parts) == 0 {
		return "", fmt.Errorf("cannot parse gitdir: %q", gitdir)
	}
	return parts[0], nil
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
