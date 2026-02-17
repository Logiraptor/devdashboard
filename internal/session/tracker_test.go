package session

import (
	"testing"
)

// stubLiveness returns a LivenessChecker that reports the given pane IDs as live.
func stubLiveness(live ...string) LivenessChecker {
	return func() (map[string]bool, error) {
		m := make(map[string]bool, len(live))
		for _, id := range live {
			m[id] = true
		}
		return m, nil
	}
}

func TestResourceKey(t *testing.T) {
	tests := []struct {
		kind     string
		repo     string
		prNumber int
		want     string
	}{
		{"repo", "devdeploy", 0, "repo:devdeploy"},
		{"pr", "devdeploy", 42, "pr:devdeploy:#42"},
		{"repo", "grafana", 0, "repo:grafana"},
		{"pr", "grafana", 7, "pr:grafana:#7"},
	}
	for _, tt := range tests {
		var got ResourceKey
		if tt.kind == "pr" && tt.prNumber > 0 {
			got = NewPRKey(tt.repo, tt.prNumber)
		} else {
			got = NewRepoKey(tt.repo)
		}
		if got.String() != tt.want {
			t.Errorf("ResourceKey(%q, %q, %d).String() = %q, want %q", tt.kind, tt.repo, tt.prNumber, got.String(), tt.want)
		}
	}
}

func TestRegisterAndQuery(t *testing.T) {
	tr := New(nil)

	key := NewRepoKey("devdeploy")
	tr.Register(key, "%1", PaneShell)
	tr.Register(key, "%2", PaneAgent)

	if tr.Count() != 2 {
		t.Errorf("Count() = %d, want 2", tr.Count())
	}

	panes := tr.PanesForResource(key)
	if len(panes) != 2 {
		t.Fatalf("PanesForResource() returned %d panes, want 2", len(panes))
	}
	if panes[0].PaneID != "%1" || panes[0].Type != PaneShell {
		t.Errorf("pane 0: got %+v, want shell %%1", panes[0])
	}
	if panes[1].PaneID != "%2" || panes[1].Type != PaneAgent {
		t.Errorf("pane 1: got %+v, want agent %%2", panes[1])
	}

	shells, agents := tr.CountForResource(key)
	if shells != 1 || agents != 1 {
		t.Errorf("CountForResource() = (%d, %d), want (1, 1)", shells, agents)
	}
}

func TestUnregister(t *testing.T) {
	tr := New(nil)

	key := NewRepoKey("devdeploy")
	tr.Register(key, "%1", PaneShell)
	tr.Register(key, "%2", PaneAgent)

	if !tr.Unregister("%1") {
		t.Error("Unregister(%1) returned false, want true")
	}
	if tr.Count() != 1 {
		t.Errorf("Count() = %d after unregister, want 1", tr.Count())
	}

	// Unregister nonexistent pane
	if tr.Unregister("%99") {
		t.Error("Unregister(%99) returned true for nonexistent pane")
	}

	// Unregister last pane for resource removes the key
	tr.Unregister("%2")
	if panes := tr.PanesForResource(key); panes != nil {
		t.Errorf("expected nil panes after removing all, got %+v", panes)
	}
}

func TestUnregisterAll(t *testing.T) {
	tr := New(nil)

	key1 := NewRepoKey("devdeploy")
	key2 := NewRepoKey("grafana")
	tr.Register(key1, "%1", PaneShell)
	tr.Register(key1, "%2", PaneAgent)
	tr.Register(key2, "%3", PaneShell)

	n := tr.UnregisterAll(key1)
	if n != 2 {
		t.Errorf("UnregisterAll() = %d, want 2", n)
	}
	if tr.Count() != 1 {
		t.Errorf("Count() = %d after UnregisterAll, want 1", tr.Count())
	}
	if panes := tr.PanesForResource(key2); len(panes) != 1 {
		t.Errorf("key2 should still have 1 pane, got %d", len(panes))
	}
}

func TestPrune(t *testing.T) {
	// Only %1 and %3 are alive; %2 is dead
	tr := New(stubLiveness("%1", "%3"))

	key1 := NewRepoKey("devdeploy")
	key2 := NewRepoKey("grafana")
	tr.Register(key1, "%1", PaneShell)
	tr.Register(key1, "%2", PaneAgent) // dead
	tr.Register(key2, "%3", PaneShell)

	pruned, err := tr.Prune()
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if pruned != 1 {
		t.Errorf("Prune() = %d, want 1", pruned)
	}
	if tr.Count() != 2 {
		t.Errorf("Count() = %d after prune, want 2", tr.Count())
	}

	// key1 should have only %1 left
	panes := tr.PanesForResource(key1)
	if len(panes) != 1 || panes[0].PaneID != "%1" {
		t.Errorf("key1 panes after prune: %+v, want [%%1]", panes)
	}
}

func TestPruneRemovesEntireResource(t *testing.T) {
	// No panes are alive
	tr := New(stubLiveness())

	key := NewRepoKey("devdeploy")
	tr.Register(key, "%1", PaneShell)
	tr.Register(key, "%2", PaneAgent)

	pruned, err := tr.Prune()
	if err != nil {
		t.Fatalf("Prune() error: %v", err)
	}
	if pruned != 2 {
		t.Errorf("Prune() = %d, want 2", pruned)
	}
	if panes := tr.PanesForResource(key); panes != nil {
		t.Errorf("expected nil panes after pruning all, got %+v", panes)
	}
}

func TestPruneNilLiveness(t *testing.T) {
	tr := New(nil)
	tr.Register(NewRepoKey("foo"), "%1", PaneShell)

	pruned, err := tr.Prune()
	if err != nil {
		t.Fatalf("Prune() with nil liveness: %v", err)
	}
	if pruned != 0 {
		t.Errorf("Prune() with nil liveness = %d, want 0", pruned)
	}
	if tr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (nil liveness should be no-op)", tr.Count())
	}
}

func TestAllPanes(t *testing.T) {
	tr := New(nil)
	tr.Register(NewRepoKey("a"), "%1", PaneShell)
	tr.Register(NewRepoKey("b"), "%2", PaneAgent)
	tr.Register(NewRepoKey("a"), "%3", PaneAgent)

	all := tr.AllPanes()
	if len(all) != 3 {
		t.Errorf("AllPanes() returned %d panes, want 3", len(all))
	}
}

func TestPanesForResourceReturnsCopy(t *testing.T) {
	tr := New(nil)
	key := NewRepoKey("devdeploy")
	tr.Register(key, "%1", PaneShell)

	panes := tr.PanesForResource(key)
	// Mutating the returned slice should not affect the tracker
	panes[0].PaneID = "%99"

	internal := tr.PanesForResource(key)
	if internal[0].PaneID != "%1" {
		t.Error("PanesForResource should return a copy, not a reference")
	}
}

func TestCountForResourceEmpty(t *testing.T) {
	tr := New(nil)
	shells, agents := tr.CountForResource(NewRepoKey("nonexistent"))
	if shells != 0 || agents != 0 {
		t.Errorf("CountForResource(nonexistent) = (%d, %d), want (0, 0)", shells, agents)
	}
}
