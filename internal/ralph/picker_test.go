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

func TestBeadPicker_Next_PrioritySorting(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "low-pri", Title: "Low priority", Status: "open", Priority: 3, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "high-pri", Title: "High priority", Status: "open", Priority: 1, CreatedAt: now},
		{ID: "mid-pri", Title: "Mid priority", Status: "open", Priority: 2, CreatedAt: now.Add(-2 * time.Hour)},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
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

func TestBeadPicker_Next_PassesEpicToCommand(t *testing.T) {
	var capturedArgs []string
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Epic:    "devdeploy-bkp",
		RunBD: func(dir string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("[]"), nil
		},
	}

	_, _ = picker.Next()

	// Verify the expected arguments were passed, including --parent epic.
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

func TestBeadPicker_Next_NoEpicWhenEmpty(t *testing.T) {
	var capturedArgs []string
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Epic:    "", // no epic filter
		RunBD: func(dir string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("[]"), nil
		},
	}

	_, _ = picker.Next()

	// Should not include --parent when epic is empty.
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

func TestBeadPicker_Count_PassesEpicToCommand(t *testing.T) {
	var capturedArgs []string
	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		Epic:    "devdeploy-bkp",
		RunBD: func(dir string, args ...string) ([]byte, error) {
			capturedArgs = args
			return []byte("[]"), nil
		},
	}

	_, _ = picker.Count()

	// Verify the expected arguments were passed, including --parent epic.
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

func TestBeadPicker_Next_SingleBead(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "only-one", Title: "Only bead", Status: "open", Priority: 2, Labels: []string{"project:p"}, CreatedAt: now},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
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

func TestDefaultScorer(t *testing.T) {
	now := time.Now()
	beads := []beads.Bead{
		{ID: "low-pri", Priority: 3, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "high-pri", Priority: 1, CreatedAt: now},
		{ID: "mid-pri", Priority: 2, CreatedAt: now.Add(-2 * time.Hour)},
	}

	DefaultScorer(beads)

	if beads[0].ID != "high-pri" {
		t.Errorf("expected high-pri first (P1), got %s (P%d)", beads[0].ID, beads[0].Priority)
	}
	if beads[1].ID != "mid-pri" {
		t.Errorf("expected mid-pri second (P2), got %s (P%d)", beads[1].ID, beads[1].Priority)
	}
	if beads[2].ID != "low-pri" {
		t.Errorf("expected low-pri third (P3), got %s (P%d)", beads[2].ID, beads[2].Priority)
	}
}

func TestDefaultScorer_SamePriority_OldestFirst(t *testing.T) {
	now := time.Now()
	beads := []beads.Bead{
		{ID: "newer", Priority: 2, CreatedAt: now},
		{ID: "oldest", Priority: 2, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "middle", Priority: 2, CreatedAt: now.Add(-24 * time.Hour)},
	}

	DefaultScorer(beads)

	if beads[0].ID != "oldest" {
		t.Errorf("expected oldest first, got %s", beads[0].ID)
	}
	if beads[1].ID != "middle" {
		t.Errorf("expected middle second, got %s", beads[1].ID)
	}
	if beads[2].ID != "newer" {
		t.Errorf("expected newer third, got %s", beads[2].ID)
	}
}

func TestComplexityScorer_PrefersLongerTitles(t *testing.T) {
	now := time.Now()
	beads := []beads.Bead{
		{ID: "short", Title: "Short", Priority: 2, CreatedAt: now},
		{ID: "very-long-title-with-more-context", Title: "Very long title with more context", Priority: 2, CreatedAt: now},
		{ID: "medium", Title: "Medium length title", Priority: 2, CreatedAt: now},
	}

	ComplexityScorer(beads)

	// Should prefer longer titles (more context)
	if beads[0].ID != "very-long-title-with-more-context" {
		t.Errorf("expected longest title first, got %s", beads[0].ID)
	}
	if beads[1].ID != "medium" {
		t.Errorf("expected medium title second, got %s", beads[1].ID)
	}
	if beads[2].ID != "short" {
		t.Errorf("expected short title third, got %s", beads[2].ID)
	}
}

func TestComplexityScorer_PrefersMoreLabels(t *testing.T) {
	now := time.Now()
	beads := []beads.Bead{
		{ID: "no-labels", Title: "Same", Priority: 2, Labels: []string{}, CreatedAt: now},
		{ID: "many-labels", Title: "Same", Priority: 2, Labels: []string{"a", "b", "c", "d"}, CreatedAt: now},
		{ID: "one-label", Title: "Same", Priority: 2, Labels: []string{"a"}, CreatedAt: now},
	}

	ComplexityScorer(beads)

	// Should prefer more labels (more context)
	if beads[0].ID != "many-labels" {
		t.Errorf("expected many-labels first, got %s", beads[0].ID)
	}
	if beads[1].ID != "one-label" {
		t.Errorf("expected one-label second, got %s", beads[1].ID)
	}
	if beads[2].ID != "no-labels" {
		t.Errorf("expected no-labels third, got %s", beads[2].ID)
	}
}

func TestComplexityScorer_PriorityTiebreaker(t *testing.T) {
	now := time.Now()
	beads := []beads.Bead{
		{ID: "low-pri", Title: "Same", Priority: 3, Labels: []string{"a"}, CreatedAt: now},
		{ID: "high-pri", Title: "Same", Priority: 1, Labels: []string{"a"}, CreatedAt: now},
		{ID: "mid-pri", Title: "Same", Priority: 2, Labels: []string{"a"}, CreatedAt: now},
	}

	ComplexityScorer(beads)

	// Same complexity, should use priority as tiebreaker
	if beads[0].ID != "high-pri" {
		t.Errorf("expected high-pri first (P1), got %s (P%d)", beads[0].ID, beads[0].Priority)
	}
	if beads[1].ID != "mid-pri" {
		t.Errorf("expected mid-pri second (P2), got %s (P%d)", beads[1].ID, beads[1].Priority)
	}
	if beads[2].ID != "low-pri" {
		t.Errorf("expected low-pri third (P3), got %s (P%d)", beads[2].ID, beads[2].Priority)
	}
}

func TestBeadPicker_Next_CustomScorer(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "short", Title: "Short", Status: "open", Priority: 2, CreatedAt: now},
		{ID: "long-title-with-more-details", Title: "Long title with more details", Status: "open", Priority: 2, CreatedAt: now},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		RunBD:   mockBDReady(entries),
		Scorer:  ComplexityScorer,
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a bead, got nil")
	}
	// ComplexityScorer prefers longer titles
	if got.ID != "long-title-with-more-details" {
		t.Errorf("expected long-title-with-more-details (longer title), got %s", got.ID)
	}
}

func TestBeadPicker_Next_DefaultScorerWhenNil(t *testing.T) {
	now := time.Now()
	entries := []bdReadyEntry{
		{ID: "low-pri", Title: "Low priority", Status: "open", Priority: 3, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "high-pri", Title: "High priority", Status: "open", Priority: 1, CreatedAt: now},
	}

	picker := &BeadPicker{
		WorkDir: "/fake/dir",
		RunBD:   mockBDReady(entries),
		Scorer:  nil, // Should default to DefaultScorer
	}

	got, err := picker.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a bead, got nil")
	}
	// DefaultScorer uses priority first
	if got.ID != "high-pri" {
		t.Errorf("expected high-pri (P1), got %s (P%d)", got.ID, got.Priority)
	}
}
