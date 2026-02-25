// Package session tracks active tmux sessions associated with project resources.
package session

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ResourceKey represents a unique identifier for a resource (repo, PR, worktree).
// It can be serialized to/from strings in the format:
//   - Repos: "repo:<name>"
//   - PRs: "pr:<repo>:#<number>"
//   - Worktrees: "worktree:<repo>:<branch>"
type ResourceKey struct {
	kind     string // "repo", "pr", or "worktree"
	repoName string
	prNumber int
	branch   string
}

// NewRepoKey creates a ResourceKey for a repository.
func NewRepoKey(repoName string) ResourceKey {
	return ResourceKey{
		kind:     "repo",
		repoName: repoName,
		prNumber: 0,
		branch:   "",
	}
}

// NewPRKey creates a ResourceKey for a pull request.
func NewPRKey(repoName string, prNumber int) ResourceKey {
	return ResourceKey{
		kind:     "pr",
		repoName: repoName,
		prNumber: prNumber,
		branch:   "",
	}
}

// NewWorktreeKey creates a ResourceKey for a non-main worktree.
func NewWorktreeKey(repoName, branch string) ResourceKey {
	return ResourceKey{
		kind:     "worktree",
		repoName: repoName,
		prNumber: 0,
		branch:   branch,
	}
}

// String returns the string representation of the resource key.
// Format: "repo:<name>" for repos, "pr:<repo>:#<number>" for PRs,
// and "worktree:<repo>:<branch>" for worktrees.
func (rk ResourceKey) String() string {
	if rk.kind == "pr" {
		return fmt.Sprintf("pr:%s:#%d", rk.repoName, rk.prNumber)
	}
	if rk.kind == "worktree" {
		return fmt.Sprintf("worktree:%s:%s", rk.repoName, rk.branch)
	}
	return fmt.Sprintf("repo:%s", rk.repoName)
}

// Kind returns the resource kind: "repo", "pr", or "worktree".
func (rk ResourceKey) Kind() string {
	return rk.kind
}

// RepoName returns the repository name.
func (rk ResourceKey) RepoName() string {
	return rk.repoName
}

// PRNumber returns the PR number. Returns 0 for repo keys.
func (rk ResourceKey) PRNumber() int {
	return rk.prNumber
}

// Branch returns the worktree branch name. Returns "" for non-worktree keys.
func (rk ResourceKey) Branch() string {
	return rk.branch
}

// IsValid checks if the resource key is valid.
// A key is valid if it has a non-empty repo name and:
// - For repos: kind is "repo"
// - For PRs: kind is "pr" and prNumber > 0
// - For worktrees: kind is "worktree" and branch is non-empty
func (rk ResourceKey) IsValid() bool {
	if rk.repoName == "" {
		return false
	}
	if rk.kind == "repo" {
		return true
	}
	if rk.kind == "pr" {
		return rk.prNumber > 0
	}
	if rk.kind == "worktree" {
		return rk.branch != ""
	}
	return false
}

// ParseResourceKey parses a string into a ResourceKey.
// Expected formats:
//   - "repo:<name>" for repositories
//   - "pr:<repo>:#<number>" for pull requests
//   - "worktree:<repo>:<branch>" for worktrees
//
// Returns an error if the string format is invalid.
func ParseResourceKey(s string) (ResourceKey, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 3)

	if len(parts) < 2 {
		return ResourceKey{}, fmt.Errorf("invalid resource key format: expected at least 2 parts separated by ':', got %q", s)
	}

	kind := parts[0]
	if kind != "repo" && kind != "pr" && kind != "worktree" {
		return ResourceKey{}, fmt.Errorf("invalid resource key kind: expected 'repo', 'pr', or 'worktree', got %q", kind)
	}

	if kind == "repo" {
		if len(parts) != 2 {
			return ResourceKey{}, fmt.Errorf("invalid repo key format: expected 'repo:<name>', got %q", s)
		}
		repoName := parts[1]
		if repoName == "" {
			return ResourceKey{}, fmt.Errorf("repo name cannot be empty")
		}
		return NewRepoKey(repoName), nil
	}

	if kind == "worktree" {
		if len(parts) != 3 {
			return ResourceKey{}, fmt.Errorf("invalid worktree key format: expected 'worktree:<repo>:<branch>', got %q", s)
		}
		repoName := parts[1]
		branch := parts[2]
		if repoName == "" {
			return ResourceKey{}, fmt.Errorf("repo name cannot be empty")
		}
		if branch == "" {
			return ResourceKey{}, fmt.Errorf("worktree branch cannot be empty")
		}
		return NewWorktreeKey(repoName, branch), nil
	}

	if len(parts) != 3 {
		return ResourceKey{}, fmt.Errorf("invalid PR key format: expected 'pr:<repo>:#<number>', got %q", s)
	}

	repoName := parts[1]
	if repoName == "" {
		return ResourceKey{}, fmt.Errorf("repo name cannot be empty")
	}

	prStr := parts[2]
	if !strings.HasPrefix(prStr, "#") {
		return ResourceKey{}, fmt.Errorf("invalid PR number format: expected '#<number>', got %q", prStr)
	}
	if strings.Contains(prStr[1:], ":") {
		return ResourceKey{}, fmt.Errorf("invalid PR key format: expected 'pr:<repo>:#<number>', got %q", s)
	}

	prNumber, err := strconv.Atoi(prStr[1:])
	if err != nil {
		return ResourceKey{}, fmt.Errorf("invalid PR number: %v", err)
	}

	if prNumber <= 0 {
		return ResourceKey{}, fmt.Errorf("PR number must be positive, got %d", prNumber)
	}

	return NewPRKey(repoName, prNumber), nil
}

var dashRunRE = regexp.MustCompile(`-+`)

// SessionName returns the deterministic tmux session name for this resource key.
// Format: "dd-" + sanitized resource URI, where ":", "#", "/" become "-", runs
// of "-" collapse to one, and leading/trailing "-" are trimmed.
func (rk ResourceKey) SessionName() string {
	uri := rk.String()
	replacer := strings.NewReplacer(":", "-", "#", "-", "/", "-")
	s := replacer.Replace(uri)
	s = dashRunRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "dd-resource"
	}
	return "dd-" + s
}
