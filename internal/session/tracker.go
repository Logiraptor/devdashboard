// Package session tracks active tmux sessions associated with project resources.
package session

import (
	"sync"
	"time"
)

// TrackedSession holds metadata about one active tmux session.
type TrackedSession struct {
	Name        string      // tmux session name (e.g. "dd-repo-devdeploy")
	ResourceKey ResourceKey // resource this session belongs to
	CreatedAt   time.Time   // when the session was registered
}

// LivenessChecker returns the set of currently live tmux session names.
// In production this calls tmux.ListSessionNames(); tests can inject a stub.
type LivenessChecker func() (map[string]bool, error)

// Tracker manages the mapping from resources to active tmux sessions.
// Safe for concurrent use.
type Tracker struct {
	mu       sync.RWMutex
	sessions map[ResourceKey]TrackedSession // resourceKey -> session
	liveness LivenessChecker
}

// New creates a Tracker with the given liveness checker.
// If liveness is nil, Prune becomes a no-op.
func New(liveness LivenessChecker) *Tracker {
	return &Tracker{
		sessions: make(map[ResourceKey]TrackedSession),
		liveness: liveness,
	}
}

// Register upserts a tracked session for the given resource.
func (t *Tracker) Register(resourceKey ResourceKey, sessionName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sessions[resourceKey] = TrackedSession{
		Name:        sessionName,
		ResourceKey: resourceKey,
		CreatedAt:   time.Now(),
	}
}

// UnregisterByName removes a tracked session by session name.
// Returns true if the session was found and removed.
func (t *Tracker) UnregisterByName(sessionName string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for key, tracked := range t.sessions {
		if tracked.Name == sessionName {
			delete(t.sessions, key)
			return true
		}
	}
	return false
}

// SessionForResource returns the tracked session for a resource key.
func (t *Tracker) SessionForResource(resourceKey ResourceKey) (TrackedSession, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.sessions[resourceKey]
	return s, ok
}

// AllSessions returns all tracked sessions across all resources.
func (t *Tracker) AllSessions() []TrackedSession {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]TrackedSession, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, s)
	}
	return out
}

// Count returns the total number of tracked sessions.
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.sessions)
}

// Prune removes dead sessions by checking liveness via tmux list-sessions.
// Returns the number of sessions pruned.
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
	for key, tracked := range t.sessions {
		if !live[tracked.Name] {
			delete(t.sessions, key)
			pruned++
		}
	}
	return pruned, nil
}

// Unregister removes tracked session info for a resource key.
// Returns true if a session was removed.
func (t *Tracker) Unregister(resourceKey ResourceKey) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.sessions[resourceKey]; ok {
		delete(t.sessions, resourceKey)
		return true
	}
	return false
}
