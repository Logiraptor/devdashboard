// Package session tracks active tmux panes associated with project resources.
// The SessionTracker maps resource keys to live panes (shells and agents),
// supports liveness pruning via tmux list-panes, and persists across project
// switches by living on AppModel rather than per-view.
package session

import (
	"sync"
	"time"
)

// PaneType distinguishes shell vs agent panes.
type PaneType string

const (
	PaneShell PaneType = "shell"
	PaneAgent PaneType = "agent"
)

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
