package worktree

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// FindWorktreeForBranch finds the worktree that has the given branch checked out.
// Returns the worktree path, or empty string if not found.
// Searches from the source repository.
//
// If excludeSrcRepo is true, the source repository path itself is excluded
// from the search results (useful when you want to find a worktree other than
// the main repo).
func FindWorktreeForBranch(srcRepo, branchName string, excludeSrcRepo bool) string {
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "list", "--porcelain")
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
			if branch == branchName && currentPath != "" {
				if excludeSrcRepo {
					// Normalize paths for comparison - resolve symlinks and make absolute
					normalizedSrc, err1 := filepath.EvalSymlinks(srcRepo)
					if err1 != nil {
						normalizedSrc, _ = filepath.Abs(srcRepo)
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
