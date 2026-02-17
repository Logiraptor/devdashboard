package worktree

import (
	"os/exec"
	"strings"
)

// FindWorktreeForBranch scans git worktree list output for a worktree
// that has the given branch checked out. Returns the worktree path or "".
// srcRepo is the path to the main git repository.
// branchName is the branch name to search for.
// excludePath optionally excludes a specific path from results (e.g., the source repo itself).
func FindWorktreeForBranch(srcRepo, branchName string, excludePath string) string {
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
				// Exclude the source repo and optionally another path
				if currentPath != srcRepo && currentPath != excludePath {
					return currentPath
				}
			}
		}
	}
	return ""
}

// WorktreeInfo represents a parsed worktree entry from git worktree list.
type WorktreeInfo struct {
	Path   string
	HEAD   string
	Branch string
}

// ParseWorktreeList parses the output of `git worktree list --porcelain`
// and returns a slice of WorktreeInfo.
func ParseWorktreeList(srcRepo string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "list", "--porcelain")
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo
	lines := strings.Split(out.String(), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			// Start of a new worktree entry
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "" && current.Path != "" {
			// Blank line indicates end of worktree entry
			worktrees = append(worktrees, current)
			current = WorktreeInfo{}
		}
	}

	// Append the last worktree if there's no trailing blank line
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}
