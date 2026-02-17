// Package worktree provides unified worktree management for both project and ralph use cases.
package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Manager provides unified worktree operations for both project and ralph use cases.
type Manager struct {
	srcRepo string // path to the main git repository
}

// Options configures worktree operations.
type Options struct {
	// UseCase determines the worktree use case: "ralph" or "project".
	// Default: "project"
	UseCase string

	// For ralph use case:
	// BaseWorkDir is the original workdir (may be a worktree itself).
	// Used to resolve srcRepo and determine base branch.
	BaseWorkDir string
	// BeadID is the bead identifier for ralph worktrees.
	// Worktree path: /tmp/ralph-<bead-id>
	// Branch name: ralph/<bead-id>
	BeadID string

	// For project use case:
	// ProjectDir is the project directory base path.
	// Worktree path: <projectDir>/<repoName>-pr-<number>
	ProjectDir string
	// RepoName is the repository name.
	RepoName string
	// PRNumber is the PR number.
	PRNumber int
	// BranchName is the PR branch name (e.g., "feat-branch").
	BranchName string
}

// WorktreeInfo holds information about a worktree.
type WorktreeInfo struct {
	Path   string // absolute path to the worktree
	Branch string // branch name checked out in the worktree
	HEAD   string // HEAD commit SHA
}

// NewManager creates a new worktree manager for the given source repository.
// The srcRepo must be the main git repository (not a worktree).
func NewManager(srcRepo string) (*Manager, error) {
	// Verify srcRepo is a git repository
	gitPath := filepath.Join(srcRepo, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("srcRepo must be the main repository, not a worktree")
	}
	return &Manager{srcRepo: srcRepo}, nil
}

// NewManagerFromWorkDir creates a new worktree manager from a workdir that may be a worktree.
// It resolves the source repository and returns a manager configured for that repo.
func NewManagerFromWorkDir(workDir string) (*Manager, error) {
	srcRepo, err := resolveSrcRepo(workDir)
	if err != nil {
		return nil, err
	}
	return NewManager(srcRepo)
}

// resolveSrcRepo resolves the source repository from a workdir that may be a worktree.
func resolveSrcRepo(workDir string) (string, error) {
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

// Create creates a worktree according to the provided options.
// Returns the worktree path and branch name.
func (m *Manager) Create(opts Options) (worktreePath string, branchName string, err error) {
	useCase := opts.UseCase
	if useCase == "" {
		useCase = "project"
	}

	switch useCase {
	case "ralph":
		return m.createRalphWorktree(opts)
	case "project":
		return m.createProjectWorktree(opts)
	default:
		return "", "", fmt.Errorf("unknown use case: %q", useCase)
	}
}

// createRalphWorktree creates a worktree for ralph use case.
func (m *Manager) createRalphWorktree(opts Options) (worktreePath string, branchName string, err error) {
	if opts.BeadID == "" {
		return "", "", fmt.Errorf("BeadID is required for ralph use case")
	}
	if opts.BaseWorkDir == "" {
		return "", "", fmt.Errorf("BaseWorkDir is required for ralph use case")
	}

	// Create worktree in /tmp/ralph-<bead-id>
	worktreePath = filepath.Join(os.TempDir(), fmt.Sprintf("ralph-%s", opts.BeadID))
	// Create a unique branch name for this worktree
	branchName = fmt.Sprintf("ralph/%s", opts.BeadID)

	// Check if worktree path already exists and is a valid worktree
	if info, err := os.Stat(worktreePath); err == nil && info.IsDir() {
		gitFile := filepath.Join(worktreePath, ".git")
		if _, err := os.Stat(gitFile); err == nil {
			// Reusing existing worktree
			return worktreePath, branchName, nil
		}
		// Path exists but is not a worktree - remove it
		_ = os.RemoveAll(worktreePath)
	}

	// Get the base branch from BaseWorkDir
	baseBranch, err := getCurrentBranch(opts.BaseWorkDir)
	if err != nil {
		return "", "", fmt.Errorf("getting current branch: %w", err)
	}

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
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", branchName, worktreePath, baseBranch)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree for %s: %s: %w", opts.BeadID, msg, err)
			}
			return "", "", fmt.Errorf("creating worktree for %s: %w", opts.BeadID, err)
		}
	} else {
		// Branch exists: check if there's already a worktree for this branch
		if existing := m.FindByBranch(branchName); existing != "" {
			return existing, branchName, nil
		}
		// Branch exists but no worktree: add worktree to existing branch
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", worktreePath, branchName)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg != "" {
				return "", "", fmt.Errorf("creating worktree for %s: %s: %w", opts.BeadID, msg, err)
			}
			return "", "", fmt.Errorf("creating worktree for %s: %w", opts.BeadID, err)
		}
	}

	return worktreePath, branchName, nil
}

// createProjectWorktree creates a worktree for project use case.
func (m *Manager) createProjectWorktree(opts Options) (worktreePath string, branchName string, err error) {
	if opts.ProjectDir == "" {
		return "", "", fmt.Errorf("ProjectDir is required for project use case")
	}
	if opts.RepoName == "" {
		return "", "", fmt.Errorf("RepoName is required for project use case")
	}
	if opts.PRNumber == 0 {
		return "", "", fmt.Errorf("PRNumber is required for project use case")
	}
	if opts.BranchName == "" {
		return "", "", fmt.Errorf("BranchName is required for project use case")
	}

	wtName := fmt.Sprintf("%s-pr-%d", opts.RepoName, opts.PRNumber)
	dstPath := filepath.Join(opts.ProjectDir, wtName)

	// Check if our target worktree path already exists and is a git worktree.
	if info, err := os.Stat(dstPath); err == nil && info.IsDir() {
		gitFile := filepath.Join(dstPath, ".git")
		if _, err := os.Stat(gitFile); err == nil {
			// Reusing existing worktree
			return dstPath, opts.BranchName, nil
		}
	}

	// Scan existing worktrees for one already on this branch.
	if existing := m.FindByBranch(opts.BranchName); existing != "" {
		// Reusing existing worktree
		return existing, opts.BranchName, nil
	}

	// Fetch the branch from origin (it may not exist locally yet).
	fetchCmd := exec.Command("git", "-C", m.srcRepo, "fetch", "origin", opts.BranchName)
	fetchCmd.Stderr = nil
	_ = fetchCmd.Run() // best-effort; branch may already be local

	// Empty dir for core.hooksPath to disable hooks during worktree add.
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(emptyHooksDir) }()
	gitNoHooks := []string{"-C", m.srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Try the local branch first; fall back to origin/<branch>.
	ref := opts.BranchName
	if err := exec.Command("git", "-C", m.srcRepo, "rev-parse", "--verify", ref).Run(); err != nil {
		ref = "origin/" + opts.BranchName
		if err := exec.Command("git", "-C", m.srcRepo, "rev-parse", "--verify", ref).Run(); err != nil {
			return "", "", fmt.Errorf("branch %s not found locally or on origin", opts.BranchName)
		}
	}

	var addStderr bytes.Buffer
	// If we have a local branch, check it out directly. If only remote, create a tracking branch.
	if ref == opts.BranchName {
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", dstPath, opts.BranchName)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", "", fmt.Errorf("git worktree add: %s", msg)
		}
	} else {
		// Create local tracking branch from origin/<branch>
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", opts.BranchName, dstPath, ref)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", "", fmt.Errorf("git worktree add: %s", msg)
		}
	}

	return dstPath, opts.BranchName, nil
}

// Remove removes a worktree at the given path.
// The worktree path must be an absolute path.
func (m *Manager) Remove(worktreePath string) error {
	// If worktree dir doesn't exist, nothing to do.
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return nil
	}

	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "remove", worktreePath, "--force")
	var stderr bytes.Buffer
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

	return nil
}

// FindByBranch finds the worktree that has the given branch checked out.
// Returns the worktree path, or empty string if not found.
func (m *Manager) FindByBranch(branchName string) string {
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
			if branch == branchName && currentPath != "" && currentPath != m.srcRepo {
				return currentPath
			}
		}
	}
	return ""
}

// List returns all worktrees for the source repository.
func (m *Manager) List() ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "-C", m.srcRepo, "worktree", "list", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo

	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			// Blank line separates worktree blocks
			if current.Path != "" && current.Path != m.srcRepo {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}

	// Don't forget the last worktree if there's no trailing blank line
	if current.Path != "" && current.Path != m.srcRepo {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// SrcRepo returns the path to the main git repository.
func (m *Manager) SrcRepo() string {
	return m.srcRepo
}
