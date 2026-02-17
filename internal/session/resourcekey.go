// Package session tracks active tmux panes associated with project resources.
package session

import (
	"fmt"
	"strconv"
	"strings"
)

// ResourceKey represents a unique identifier for a project resource (repo or PR).
// It can be serialized to/from strings in the format:
//   - Repos: "repo:<name>"
//   - PRs: "pr:<repo>:#<number>"
type ResourceKey struct {
	kind      string // "repo" or "pr"
	repoName  string
	prNumber  int
}

// NewRepoKey creates a ResourceKey for a repository.
func NewRepoKey(repoName string) ResourceKey {
	return ResourceKey{
		kind:     "repo",
		repoName: repoName,
		prNumber: 0,
	}
}

// NewPRKey creates a ResourceKey for a pull request.
func NewPRKey(repoName string, prNumber int) ResourceKey {
	return ResourceKey{
		kind:     "pr",
		repoName: repoName,
		prNumber: prNumber,
	}
}

// String returns the string representation of the resource key.
// Format: "repo:<name>" for repos, "pr:<repo>:#<number>" for PRs.
func (rk ResourceKey) String() string {
	if rk.kind == "pr" && rk.prNumber > 0 {
		return fmt.Sprintf("pr:%s:#%d", rk.repoName, rk.prNumber)
	}
	return fmt.Sprintf("repo:%s", rk.repoName)
}

// Kind returns the resource kind: "repo" or "pr".
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

// IsValid checks if the resource key is valid.
// A key is valid if it has a non-empty repo name and:
// - For repos: kind is "repo"
// - For PRs: kind is "pr" and prNumber > 0
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
	return false
}

// ParseResourceKey parses a string into a ResourceKey.
// Expected formats:
//   - "repo:<name>" for repositories
//   - "pr:<repo>:#<number>" for pull requests
//
// Returns an error if the string format is invalid.
func ParseResourceKey(s string) (ResourceKey, error) {
	// Remove any leading/trailing whitespace
	s = strings.TrimSpace(s)
	
	// Split by colon
	parts := strings.Split(s, ":")
	
	if len(parts) < 2 {
		return ResourceKey{}, fmt.Errorf("invalid resource key format: expected at least 2 parts separated by ':', got %q", s)
	}
	
	kind := parts[0]
	if kind != "repo" && kind != "pr" {
		return ResourceKey{}, fmt.Errorf("invalid resource key kind: expected 'repo' or 'pr', got %q", kind)
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
	
	// PR format: "pr:<repo>:#<number>"
	if len(parts) != 3 {
		return ResourceKey{}, fmt.Errorf("invalid PR key format: expected 'pr:<repo>:#<number>', got %q", s)
	}
	
	repoName := parts[1]
	if repoName == "" {
		return ResourceKey{}, fmt.Errorf("repo name cannot be empty")
	}
	
	// Parse PR number (should start with #)
	prStr := parts[2]
	if !strings.HasPrefix(prStr, "#") {
		return ResourceKey{}, fmt.Errorf("invalid PR number format: expected '#<number>', got %q", prStr)
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
