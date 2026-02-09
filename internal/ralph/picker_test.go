package ralph

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"devdeploy/internal/beads"
)

// mockBDReady returns a RunBDFunc that serves canned JSON responses.
func mockBDReady(entries []bdReadyEntry) RunBDFunc {
	return func(dir string, args ...string) ([]byte, error) {
		data, err := json.Marshal(entries)
		return data, err
	}
}

// mockBDError returns a RunBDFunc that always returns an error.
func mockBDError(msg string) RunBDFunc {
	return func(dir string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

func TestBeadPicker_Next_PrioritySorting(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "low-pri", Title: "Low priority", Status: "open", Priority: 3, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "high-pri", Title: "High priority", Status: "open", Priority: 1, CreatedAt: now},
		{ID: "mid-pri", Title: "Mid priority", Status: "open", Priority: 2, CreatedAt: now.Add(-2 * time.Hour)},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "myproj",
		RunBD:   mockBDReady(entries),
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a bead, got nil")
	}
	if got.ID != "high-pri" {
		t.Errorf("expected bead ID high-pri (P1), got %s (P%d)", got.ID, got.Priority)
	}
}

func TestBeadPicker_Next_SamePriority_OldestFirst(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "newer", Title: "Newer bead", Status: "open", Priority: 2, CreatedAt: now},
		{ID: "oldest", Title: "Oldest bead", Status: "open", Priority: 2, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "middle", Title: "Middle bead", Status: "open", Priority: 2, CreatedAt: now.Add(-24 * time.Hour)},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "myproj",
		RunBD:   mockBDReady(entries),
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a bead, got nil")
	}
	if got.ID != "oldest" {
		t.Errorf("expected oldest bead (oldest), got %s", got.ID)
	}
}

func TestBeadPicker_Next_NoBeadsAvailable(t *testing.T) {
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "myproj",
		RunBD:   mockBDReady([]bdReadyEntry{}),
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when no beads available, got %+v", got)
	}
}

func TestBeadPicker_Next_BDError(t *testing.T) {
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "myproj",
		RunBD:   mockBDError("bd not found"),
	}

	got, err := picker.Next()
	if err == nil {
		t.Fatal("expected error when bd fails, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bead on error, got %+v", got)
	}
}

func TestBeadPicker_Next_InvalidJSON(t *testing.T) {
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "myproj",
		RunBD: func(dir string, args ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
	}

	got, err := picker.Next()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bead on parse error, got %+v", got)
	}
}

func TestBeadPicker_Next_PassesLabelsToCommand(t *testing.T) {
	var capturedArgs []string
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "testproj",
		Labels:  []string{"team:backend", "scope:api"},
		RunBD: func(dir string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("[]"), nil
		},
	}

	_, _ = picker.Next()

	// Verify the expected arguments were passed.
	expected := []string{"ready", "--json", "--label", "project:testproj", "--label", "team:backend", "--label", "scope:api"}
	if len(capturedArgs) != len(expected) {
		t.Fatalf("expected args %v, got %v", expected, capturedArgs)
	}
	for i, want := range expected {
		if capturedArgs[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, capturedArgs[i])
		}
	}
}

func TestBeadPicker_Next_NoProjectLabel(t *testing.T) {
	var capturedArgs []string
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "", // no project filter
		RunBD: func(dir string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("[]"), nil
		},
	}

	_, _ = picker.Next()

	// Should not include --label project:... when project is empty.
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

func TestBeadPicker_Next_SingleBead(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "only-one", Title: "Only bead", Status: "open", Priority: 2, Labels: []string{"project:p"}, CreatedAt: now},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "p",
		RunBD:   mockBDReady(entries),
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a bead, got nil")
	}
	if got.ID != "only-one" {
		t.Errorf("expected bead only-one, got %s", got.ID)
	}
}

func TestBeadPicker_Next_ReturnsBeadFields(t *testing.T) {
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

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Project: "test",
		RunBD:   mockBDReady(entries),
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := &beads.Bead{
		ID:        "full-bead",
		Title:     "Full field bead",
		Status:    "open",
		Priority:  1,
		Labels:    []string{"project:test", "team:core"},
		CreatedAt: now,
	}

	if got.ID != want.ID || got.Title != want.Title || got.Status != want.Status ||
		got.Priority != want.Priority || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("bead fields mismatch:\ngot:  %+v\nwant: %+v", got, want)
	}
	if len(got.Labels) != len(want.Labels) {
		t.Errorf("labels length mismatch: got %d, want %d", len(got.Labels), len(want.Labels))
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
