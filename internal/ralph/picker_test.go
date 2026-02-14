package ralph

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// mockBDReady returns a BDRunner that serves canned JSON responses.
func mockBDReady(entries []bdReadyEntry) BDRunner {
	return func(dir string, args ...string) ([]byte, error) {
		data, err := json.Marshal(entries)
		return data, err
	}
}

// mockBDError returns a BDRunner that always returns an error.
func mockBDError(msg string) BDRunner {
	return func(dir string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

func TestReadyBeads_PrioritySorting(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "low-pri", Title: "Low priority", Status: "open", Priority: 3, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "high-pri", Title: "High priority", Status: "open", Priority: 1, CreatedAt: now},
		{ID: "mid-pri", Title: "Mid priority", Status: "open", Priority: 2, CreatedAt: now.Add(-2 * time.Hour)},
	}

	got, err := ReadyBeadsWithRunner(mockBDReady(entries), "/fake/dir", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 beads, got %d", len(got))
	}
	if got[0].ID != "high-pri" {
		t.Errorf("expected first bead ID high-pri (P1), got %s (P%d)", got[0].ID, got[0].Priority)
	}
}

func TestReadyBeads_SamePriority_OldestFirst(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "newer", Title: "Newer bead", Status: "open", Priority: 2, CreatedAt: now},
		{ID: "oldest", Title: "Oldest bead", Status: "open", Priority: 2, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "middle", Title: "Middle bead", Status: "open", Priority: 2, CreatedAt: now.Add(-24 * time.Hour)},
	}

	got, err := ReadyBeadsWithRunner(mockBDReady(entries), "/fake/dir", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 beads, got %d", len(got))
	}
	if got[0].ID != "oldest" {
		t.Errorf("expected oldest bead first, got %s", got[0].ID)
	}
}

func TestReadyBeads_NoBeadsAvailable(t *testing.T) {
	got, err := ReadyBeadsWithRunner(mockBDReady([]bdReadyEntry{}), "/fake/dir", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice when no beads available, got %+v", got)
	}
}

func TestReadyBeads_BDError(t *testing.T) {
	got, err := ReadyBeadsWithRunner(mockBDError("bd not found"), "/fake/dir", "")
	if err == nil {
		t.Fatal("expected error when bd fails, got nil")
	}
	if got != nil {
		t.Errorf("expected nil slice on error, got %+v", got)
	}
}

func TestReadyBeads_InvalidJSON(t *testing.T) {
	runner := func(dir string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	got, err := ReadyBeadsWithRunner(runner, "/fake/dir", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if got != nil {
		t.Errorf("expected nil slice on parse error, got %+v", got)
	}
}

func TestReadyBeads_PassesParentToCommand(t *testing.T) {
	var capturedArgs []string
	runner := func(dir string, args ...string) ([]byte, error) {
		capturedArgs = args
		return []byte("[]"), nil
	}

	_, _ = ReadyBeadsWithRunner(runner, "/fake/dir", "devdeploy-bkp")

	expected := []string{"ready", "--json", "--parent", "devdeploy-bkp"}
	if len(capturedArgs) != len(expected) {
		t.Fatalf("expected args %v, got %v", expected, capturedArgs)
	}
	for i, want := range expected {
		if capturedArgs[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, capturedArgs[i])
		}
	}
}

func TestReadyBeads_NoParentWhenEmpty(t *testing.T) {
	var capturedArgs []string
	runner := func(dir string, args ...string) ([]byte, error) {
		capturedArgs = args
		return []byte("[]"), nil
	}

	_, _ = ReadyBeadsWithRunner(runner, "/fake/dir", "")

	expected := []string{"ready", "--json"}
	if len(capturedArgs) != len(expected) {
		t.Fatalf("expected args %v, got %v", expected, capturedArgs)
	}
	for i, want := range expected {
		if capturedArgs[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, capturedArgs[i])
		}
	}
}

func TestReadyBeads_SingleBead(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "only-one", Title: "Only bead", Status: "open", Priority: 2, Labels: []string{"project:p"}, CreatedAt: now},
	}

	got, err := ReadyBeadsWithRunner(mockBDReady(entries), "/fake/dir", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(got))
	}
	if got[0].ID != "only-one" {
		t.Errorf("expected bead only-one, got %s", got[0].ID)
	}
}

func TestReadyBeads_ReturnsBeadFields(t *testing.T) {
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	entries := []bdReadyEntry{
		{
			ID:        "full-bead",
			Title:     "Full field bead",
			Status:    "open",
			Priority:  1,
			Labels:    []string{"project:test", "team:core"},
			CreatedAt: now,
		},
	}

	got, err := ReadyBeadsWithRunner(mockBDReady(entries), "/fake/dir", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(got))
	}

	want := beads.Bead{
		ID:        "full-bead",
		Title:     "Full field bead",
		Status:    "open",
		Priority:  1,
		Labels:    []string{"project:test", "team:core"},
		CreatedAt: now,
	}

	if got[0].ID != want.ID || got[0].Title != want.Title || got[0].Status != want.Status ||
		got[0].Priority != want.Priority || !got[0].CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("bead fields mismatch:\ngot:  %+v\nwant: %+v", got[0], want)
	}
	if len(got[0].Labels) != len(want.Labels) {
		t.Errorf("labels length mismatch: got %d, want %d", len(got[0].Labels), len(want.Labels))
	}
}

func TestParseReadyBeads_EmptyArray(t *testing.T) {
	got, err := parseReadyBeads([]byte("[]"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %+v", got)
	}
}

func TestParseReadyBeads_InvalidJSON(t *testing.T) {
	_, err := parseReadyBeads([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
