package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveCommonDir returns the git common directory for a worktree.
// Git reads info/exclude from the common dir, not the per-worktree gitdir.
//
// For a regular repo (.git is a directory), the common dir is .git/ itself.
// For a worktree (.git is a file with "gitdir: <path>"), the per-worktree
// gitdir contains a "commondir" file with a relative path to the shared
// git directory.
func ResolveCommonDir(worktreePath string) (string, error) {
	dotGit := filepath.Join(worktreePath, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", err
	}
	// Regular repo: .git is a directory and is itself the common dir.
	if info.IsDir() {
		return dotGit, nil
	}

	// Worktree: .git is a file with "gitdir: <path>"
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf(".git file has unexpected format: %s", line)
	}
	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktreePath, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	// Read the "commondir" file to find the shared git directory.
	// This file contains a relative path (typically "../..") from
	// the worktree gitdir to the main repo's .git/.
	commonDirFile := filepath.Join(gitDir, "commondir")
	cdData, err := os.ReadFile(commonDirFile)
	if err != nil {
		// No commondir file — fall back to the gitdir itself.
		// This shouldn't happen for real worktrees but is safe.
		return gitDir, nil
	}
	commonRel := strings.TrimSpace(string(cdData))
	if !filepath.IsAbs(commonRel) {
		commonRel = filepath.Join(gitDir, commonRel)
	}
	return filepath.Clean(commonRel), nil
}
