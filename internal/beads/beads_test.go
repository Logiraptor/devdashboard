package beads

import (
	"encoding/json"
	"fmt"
	"testing"
)

// mockBD replaces runBD for testing. Returns canned output keyed by the
// label arguments so we can distinguish repo vs PR queries.
func mockBD(responses map[string][]bdListEntry) func(string, ...string) ([]byte, error) {
	return func(dir string, args ...string) ([]byte, error) {
		// Build a key from the args for lookup.
		key := fmt.Sprintf("%v", args)
		for k, v := range responses {
			if key == k || containsAll(args, k) {
				data, err := json.Marshal(v)
				return data, err
			}
		}
		// No matching response — simulate bd not found.
		return nil, fmt.Errorf("no mock response for %v", args)
	}
}

// containsAll checks if all space-separated tokens in want appear in args.
func containsAll(args []string, want string) bool {
	joined := fmt.Sprintf("%v", args)
	return joined == want
}

func TestListForRepo_FiltersClosedAndPRBeads(t *testing.T) {
	entries := []bdListEntry{
		{ID: "a-1", Title: "Open repo bead", Status: "open", Priority: 2, Labels: []string{"project:myproj"}},
		{ID: "a-2", Title: "In-progress bead", Status: "in_progress", Priority: 1, Labels: []string{"project:myproj"}},
		{ID: "a-3", Title: "Closed bead", Status: "closed", Priority: 2, Labels: []string{"project:myproj"}},
		{ID: "a-4", Title: "PR bead", Status: "open", Priority: 2, Labels: []string{"project:myproj", "pr:42"}},
	}

	key := fmt.Sprintf("%v", []string{"list", "--label", "project:myproj", "--json", "--limit", "0"})
	old := runBD
	runBD = mockBD(map[string][]bdListEntry{key: entries})
	defer func() { runBD = old }()

	got := ListForRepo("/fake/dir", "myproj")

	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %d: %+v", len(got), got)
	}
	if got[0].ID != "a-1" {
		t.Errorf("expected first bead ID a-1, got %s", got[0].ID)
	}
	if got[1].ID != "a-2" {
		t.Errorf("expected second bead ID a-2, got %s", got[1].ID)
	}
}

func TestListForPR(t *testing.T) {
	entries := []bdListEntry{
		{ID: "b-1", Title: "PR bead", Status: "open", Priority: 1, Labels: []string{"project:myproj", "pr:7"}},
		{ID: "b-2", Title: "Closed PR bead", Status: "closed", Priority: 2, Labels: []string{"project:myproj", "pr:7"}},
	}

	key := fmt.Sprintf("%v", []string{"list", "--label", "project:myproj", "--label", "pr:7", "--json", "--limit", "0"})
	old := runBD
	runBD = mockBD(map[string][]bdListEntry{key: entries})
	defer func() { runBD = old }()

	got := ListForPR("/fake/dir", "myproj", 7)

	if len(got) != 1 {
		t.Fatalf("expected 1 bead, got %d: %+v", len(got), got)
	}
	if got[0].ID != "b-1" {
		t.Errorf("expected bead ID b-1, got %s", got[0].ID)
	}
}

func TestListForRepo_BDNotAvailable(t *testing.T) {
	old := runBD
	runBD = func(dir string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exec: \"bd\": executable file not found in $PATH")
	}
	defer func() { runBD = old }()

	got := ListForRepo("/fake/dir", "myproj")
	if got != nil {
		t.Errorf("expected nil when bd unavailable, got %+v", got)
	}
}

func TestListForPR_BDNotAvailable(t *testing.T) {
	old := runBD
	runBD = func(dir string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exec: \"bd\": executable file not found in $PATH")
	}
	defer func() { runBD = old }()

	got := ListForPR("/fake/dir", "myproj", 42)
	if got != nil {
		t.Errorf("expected nil when bd unavailable, got %+v", got)
	}
}

func TestParseBeads_InvalidJSON(t *testing.T) {
	got := parseBeads([]byte("not json"))
	if got != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestParseBeads_EmptyArray(t *testing.T) {
	got := parseBeads([]byte("[]"))
	if len(got) != 0 {
		t.Errorf("expected empty slice for [], got %+v", got)
	}
}

func TestHasPRLabel(t *testing.T) {
	tests := []struct {
		labels []string
		want   bool
	}{
		{nil, false},
		{[]string{"project:foo"}, false},
		{[]string{"pr:1"}, true},
		{[]string{"project:foo", "pr:99"}, true},
	}
	for _, tt := range tests {
		if got := hasPRLabel(tt.labels); got != tt.want {
			t.Errorf("hasPRLabel(%v) = %v, want %v", tt.labels, got, tt.want)
		}
	}
}
