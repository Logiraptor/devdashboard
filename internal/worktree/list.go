package worktree

import (
	"os/exec"
	"strings"
)

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
