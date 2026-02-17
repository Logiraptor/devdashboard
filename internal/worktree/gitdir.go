package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSourceRepo resolves the source repository path from a worktree path.
// If workDir is a worktree, it reads the .git file to find the main repo.
// If workDir is the main repo, it returns workDir.
// Returns an error if workDir is not a git repository.
func ResolveSourceRepo(workDir string) (string, error) {
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

// ParseGitDir parses a .git file content and returns the gitdir path.
// The input should be the content of a .git file (e.g., "gitdir: /path/to/.git/worktrees/name").
// Returns the gitdir path without the "gitdir: " prefix.
func ParseGitDir(content string) (string, error) {
	gitdir := strings.TrimSpace(content)
	if !strings.HasPrefix(gitdir, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format: %q", gitdir)
	}
	return strings.TrimPrefix(gitdir, "gitdir: "), nil
}
