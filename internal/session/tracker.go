// Package session tracks active tmux panes associated with project resources.
// The SessionTracker maps resource keys to live panes (shells and agents),
// supports liveness pruning via tmux list-panes, and persists across project
// switches by living on AppModel rather than per-view.
package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// PaneType distinguishes shell vs agent panes.
type PaneType string

const (
	PaneShell PaneType = "shell"
	PaneAgent PaneType = "agent"
)

// ResourceKey represents a canonical key for a resource.
// It can represent either a repo resource or a PR resource.
type ResourceKey struct {
	kind     string // "repo" or "pr"
	repoName string
	prNumber int
}

// Kind returns the resource kind ("repo" or "pr").
func (rk ResourceKey) Kind() string {
	return rk.kind
}

// RepoName returns the repository name.
func (rk ResourceKey) RepoName() string {
	return rk.repoName
}

// PRNumber returns the PR number (0 for repo resources).
func (rk ResourceKey) PRNumber() int {
	return rk.prNumber
}

// String returns the canonical string representation of the resource key.
// Repos: "repo:<name>", PRs: "pr:<repo>:#<number>".
func (rk ResourceKey) String() string {
	if rk.kind == "pr" && rk.prNumber > 0 {
		return fmt.Sprintf("pr:%s:#%d", rk.repoName, rk.prNumber)
	}
	return fmt.Sprintf("repo:%s", rk.repoName)
}

// ParseResourceKey parses a string resource key into a ResourceKey struct.
// Returns an error if the format is invalid.
func ParseResourceKey(s string) (ResourceKey, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return ResourceKey{}, fmt.Errorf("invalid resource key format: %q", s)
	}
	kind := parts[0]
	repoName := parts[1]
	if kind == "pr" && len(parts) >= 3 {
		// Parse PR number from "pr:repo:#42"
		prStr := strings.TrimPrefix(parts[2], "#")
		var prNumber int
		if _, err := fmt.Sscanf(prStr, "%d", &prNumber); err != nil {
			return ResourceKey{}, fmt.Errorf("invalid PR number in resource key: %q", s)
		}
		return ResourceKey{kind: "pr", repoName: repoName, prNumber: prNumber}, nil
	}
	return ResourceKey{kind: "repo", repoName: repoName, prNumber: 0}, nil
}

// NewResourceKey creates a ResourceKey struct.
// Repos: kind="repo", prNumber=0. PRs: kind="pr", prNumber>0.
func NewResourceKey(kind string, repoName string, prNumber int) ResourceKey {
	return ResourceKey{kind: kind, repoName: repoName, prNumber: prNumber}
}

// ResourceKey builds a canonical key for a resource (legacy function for backward compatibility).
// Repos: "repo:<name>", PRs: "pr:<repo>:#<number>".
// Deprecated: Use NewResourceKey instead.
func ResourceKeyString(kind string, repoName string, prNumber int) string {
	return NewResourceKey(kind, repoName, prNumber).String()
}

// TrackedPane holds metadata about one active tmux pane.
type TrackedPane struct {
	PaneID      string      // tmux pane ID (e.g. "%42")
	Type        PaneType    // shell or agent
	ResourceKey ResourceKey // resource this pane belongs to
	CreatedAt   time.Time   // when the pane was registered
}

// LivenessChecker returns the set of currently live tmux pane IDs.
// In production this calls tmux.ListPaneIDs(); tests can inject a stub.
type LivenessChecker func() (map[string]bool, error)

// Tracker manages the mapping from resources to active tmux panes.
// Safe for concurrent use.
type Tracker struct {
	mu       sync.RWMutex
	panes    map[ResourceKey][]TrackedPane // resourceKey -> panes
	liveness LivenessChecker
}

// New creates a Tracker with the given liveness checker.
// If liveness is nil, Prune becomes a no-op.
func New(liveness LivenessChecker) *Tracker {
	return &Tracker{
		panes:    make(map[ResourceKey][]TrackedPane),
		liveness: liveness,
	}
}

// Register adds a pane to the tracker for the given resource.
func (t *Tracker) Register(resourceKey ResourceKey, paneID string, paneType PaneType) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.panes[resourceKey] = append(t.panes[resourceKey], TrackedPane{
		PaneID:      paneID,
		Type:        paneType,
		ResourceKey: resourceKey,
		CreatedAt:   time.Now(),
	})
}

// Unregister removes a specific pane by ID from the tracker.
// Returns true if the pane was found and removed.
func (t *Tracker) Unregister(paneID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, panes := range t.panes {
		for i, p := range panes {
			if p.PaneID == paneID {
				t.panes[key] = append(panes[:i], panes[i+1:]...)
				if len(t.panes[key]) == 0 {
					delete(t.panes, key)
				}
				return true
			}
		}
	}
	return false
}

// PanesForResource returns tracked panes for a resource key.
// Returns nil if no panes are tracked.
func (t *Tracker) PanesForResource(resourceKey ResourceKey) []TrackedPane {
	t.mu.RLock()
	defer t.mu.RUnlock()
	panes := t.panes[resourceKey]
	if len(panes) == 0 {
		return nil
	}
	out := make([]TrackedPane, len(panes))
	copy(out, panes)
	return out
}

// AllPanes returns all tracked panes across all resources.
func (t *Tracker) AllPanes() []TrackedPane {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []TrackedPane
	for _, panes := range t.panes {
		out = append(out, panes...)
	}
	return out
}

// Count returns the total number of tracked panes.
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := 0
	for _, panes := range t.panes {
		n += len(panes)
	}
	return n
}

// CountForResource returns (shells, agents) for a given resource key.
func (t *Tracker) CountForResource(resourceKey ResourceKey) (shells, agents int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, p := range t.panes[resourceKey] {
		switch p.Type {
		case PaneShell:
			shells++
		case PaneAgent:
			agents++
		}
	}
	return
}

// Prune removes dead panes by checking liveness via tmux list-panes.
// Returns the number of panes pruned.
func (t *Tracker) Prune() (int, error) {
	if t.liveness == nil {
		return 0, nil
	}
	live, err := t.liveness()
	if err != nil {
		return 0, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	pruned := 0
	for key, panes := range t.panes {
		var kept []TrackedPane
		for _, p := range panes {
			if live[p.PaneID] {
				kept = append(kept, p)
			} else {
				pruned++
			}
		}
		if len(kept) == 0 {
			delete(t.panes, key)
		} else {
			t.panes[key] = kept
		}
	}
	return pruned, nil
}

// UnregisterAll removes all panes for a resource key.
// Returns the number of panes removed.
func (t *Tracker) UnregisterAll(resourceKey ResourceKey) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(t.panes[resourceKey])
	delete(t.panes, resourceKey)
	return n
}
