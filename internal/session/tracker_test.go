package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubLiveness returns a LivenessChecker that reports the given session names as live.
func stubLiveness(live ...string) LivenessChecker {
	return func() (map[string]bool, error) {
		m := make(map[string]bool, len(live))
		for _, id := range live {
			m[id] = true
		}
		return m, nil
	}
}

func TestRegisterAndQuery(t *testing.T) {
	tr := New(nil)
	key := NewRepoKey("devdeploy")

	tr.Register(key, "dd-repo-devdeploy")
	require.Equal(t, 1, tr.Count())

	s, ok := tr.SessionForResource(key)
	require.True(t, ok)
	require.Equal(t, "dd-repo-devdeploy", s.Name)
}

func TestUnregisterByResourceKey(t *testing.T) {
	tr := New(nil)
	key := NewRepoKey("devdeploy")

	tr.Register(key, "dd-repo-devdeploy")
	require.True(t, tr.UnregisterByName("dd-repo-devdeploy"))
	require.Equal(t, 0, tr.Count())
	_, ok := tr.SessionForResource(key)
	require.False(t, ok)
	require.False(t, tr.UnregisterByName("dd-does-not-exist"))
}

func TestUnregister(t *testing.T) {
	tr := New(nil)

	key1 := NewRepoKey("devdeploy")
	key2 := NewRepoKey("grafana")
	tr.Register(key1, "dd-repo-devdeploy")
	tr.Register(key2, "dd-repo-grafana")

	removed := tr.Unregister(key1)
	require.True(t, removed)
	require.Equal(t, 1, tr.Count())
	_, ok := tr.SessionForResource(key2)
	require.True(t, ok)
	require.False(t, tr.Unregister(key1))
}

func TestPrune(t *testing.T) {
	tr := New(stubLiveness("dd-repo-devdeploy"))

	key1 := NewRepoKey("devdeploy")
	key2 := NewRepoKey("grafana")
	tr.Register(key1, "dd-repo-devdeploy")
	tr.Register(key2, "dd-repo-grafana")

	pruned, err := tr.Prune()
	require.NoError(t, err)
	require.Equal(t, 1, pruned)
	require.Equal(t, 1, tr.Count())
	s, ok := tr.SessionForResource(key1)
	require.True(t, ok)
	require.Equal(t, "dd-repo-devdeploy", s.Name)
}

func TestPruneRemovesEntireResource(t *testing.T) {
	tr := New(stubLiveness())

	key := NewRepoKey("devdeploy")
	tr.Register(key, "dd-repo-devdeploy")

	pruned, err := tr.Prune()
	require.NoError(t, err)
	require.Equal(t, 1, pruned)
	_, ok := tr.SessionForResource(key)
	require.False(t, ok)
}

func TestPruneNilLiveness(t *testing.T) {
	tr := New(nil)
	tr.Register(NewRepoKey("foo"), "dd-repo-foo")

	pruned, err := tr.Prune()
	require.NoError(t, err)
	require.Equal(t, 0, pruned)
	require.Equal(t, 1, tr.Count())
}

func TestPruneLivenessError(t *testing.T) {
	tr := New(func() (map[string]bool, error) {
		return nil, errors.New("liveness failed")
	})
	tr.Register(NewRepoKey("foo"), "dd-repo-foo")

	pruned, err := tr.Prune()
	require.Error(t, err)
	require.Equal(t, 0, pruned)
	require.Equal(t, 1, tr.Count())
}

func TestAllSessions(t *testing.T) {
	tr := New(nil)
	tr.Register(NewRepoKey("a"), "dd-repo-a")
	tr.Register(NewRepoKey("b"), "dd-repo-b")

	all := tr.AllSessions()
	require.Len(t, all, 2)
}

func TestSessionForResourceValueCopy(t *testing.T) {
	tr := New(nil)
	key := NewRepoKey("devdeploy")
	tr.Register(key, "dd-repo-devdeploy")

	got, ok := tr.SessionForResource(key)
	require.True(t, ok)
	got.Name = "mutated-locally"
	internal, ok := tr.SessionForResource(key)
	require.True(t, ok)
	require.Equal(t, "dd-repo-devdeploy", internal.Name)
}
