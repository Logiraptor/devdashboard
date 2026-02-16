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
		{ID: "a-1", Title: "Open repo bead", Status: "open", Priority: 2, Labels: []string{}},
		{ID: "a-2", Title: "In-progress bead", Status: "in_progress", Priority: 1, Labels: []string{}},
		{ID: "a-3", Title: "Closed bead", Status: "closed", Priority: 2, Labels: []string{}},
		{ID: "a-4", Title: "PR bead", Status: "open", Priority: 2, Labels: []string{"pr:42"}},
	}

	key := fmt.Sprintf("%v", []string{"list", "--json", "--limit", "0"})
	old := runBD
	runBD = mockBD(map[string][]bdListEntry{key: entries})
	defer func() { runBD = old }()

	got, err := ListForRepo("/fake/dir", "myproj")
	if err != nil {
		t.Fatalf("ListForRepo returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %d: %+v", len(got), got)
	}
	// SortHierarchically sorts standalone beads by priority (ascending) then ID.
	// a-2 has priority 1, a-1 has priority 2.
	if got[0].ID != "a-2" {
		t.Errorf("expected first bead ID a-2 (priority 1), got %s", got[0].ID)
	}
	if got[1].ID != "a-1" {
		t.Errorf("expected second bead ID a-1 (priority 2), got %s", got[1].ID)
	}
}

func TestListForPR(t *testing.T) {
	entries := []bdListEntry{
		{ID: "b-1", Title: "PR bead", Status: "open", Priority: 1, Labels: []string{"pr:7"}},
		{ID: "b-2", Title: "Closed PR bead", Status: "closed", Priority: 2, Labels: []string{"pr:7"}},
	}

	key := fmt.Sprintf("%v", []string{"list", "--label", "pr:7", "--json", "--limit", "0"})
	old := runBD
	runBD = mockBD(map[string][]bdListEntry{key: entries})
	defer func() { runBD = old }()

	got, err := ListForPR("/fake/dir", "myproj", 7)
	if err != nil {
		t.Fatalf("ListForPR returned error: %v", err)
	}

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

	got, err := ListForRepo("/fake/dir", "myproj")
	if err == nil {
		t.Errorf("expected error when bd unavailable, got nil")
	}
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

	got, err := ListForPR("/fake/dir", "myproj", 42)
	if err == nil {
		t.Errorf("expected error when bd unavailable, got nil")
	}
	if got != nil {
		t.Errorf("expected nil when bd unavailable, got %+v", got)
	}
}

func TestParseBeads_InvalidJSON(t *testing.T) {
	got, err := parseBeads([]byte("not json"))
	if err == nil {
		t.Errorf("expected error for invalid JSON, got nil")
	}
	if got != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestParseBeads_EmptyArray(t *testing.T) {
	got, err := parseBeads([]byte("[]"))
	if err != nil {
		t.Fatalf("parseBeads returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice for [], got %+v", got)
	}
}

func TestExtractParentID(t *testing.T) {
	tests := []struct {
		name string
		deps []bdDependency
		want string
	}{
		{"nil deps", nil, ""},
		{"empty deps", []bdDependency{}, ""},
		{"blocks only", []bdDependency{{IssueID: "a", DependsOnID: "b", Type: "blocks"}}, ""},
		{"parent-child", []bdDependency{{IssueID: "child-1", DependsOnID: "epic-1", Type: "parent-child"}}, "epic-1"},
		{"mixed deps", []bdDependency{
			{IssueID: "child-1", DependsOnID: "other", Type: "blocks"},
			{IssueID: "child-1", DependsOnID: "epic-1", Type: "parent-child"},
		}, "epic-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractParentID(tt.deps); got != tt.want {
				t.Errorf("extractParentID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortHierarchically_Empty(t *testing.T) {
	got := SortHierarchically(nil)
	if got != nil {
		t.Errorf("expected nil for nil input, got %+v", got)
	}
	got = SortHierarchically([]Bead{})
	if len(got) != 0 {
		t.Errorf("expected empty for empty input, got %+v", got)
	}
}

func TestSortHierarchically_SingleBead(t *testing.T) {
	input := []Bead{{ID: "x-1", Title: "Solo", IssueType: "task"}}
	got := SortHierarchically(input)
	if len(got) != 1 || got[0].ID != "x-1" {
		t.Errorf("expected single bead unchanged, got %+v", got)
	}
}

func TestSortHierarchically_EpicWithChildren(t *testing.T) {
	input := []Bead{
		{ID: "child-2", Title: "Child 2", IssueType: "task", ParentID: "epic-1", Priority: 2},
		{ID: "standalone", Title: "Standalone", IssueType: "task", Priority: 1},
		{ID: "epic-1", Title: "Epic 1", IssueType: "epic", Priority: 1},
		{ID: "child-1", Title: "Child 1", IssueType: "task", ParentID: "epic-1", Priority: 1},
	}

	got := SortHierarchically(input)

	wantOrder := []string{"epic-1", "child-1", "child-2", "standalone"}
	if len(got) != len(wantOrder) {
		t.Fatalf("expected %d beads, got %d: %+v", len(wantOrder), len(got), got)
	}
	for i, wantID := range wantOrder {
		if got[i].ID != wantID {
			t.Errorf("position %d: expected %s, got %s", i, wantID, got[i].ID)
		}
	}
}

func TestSortHierarchically_MultipleEpics(t *testing.T) {
	input := []Bead{
		{ID: "task-standalone", Title: "Standalone task", IssueType: "task", Priority: 3},
		{ID: "epic-b.1", Title: "Epic B child", IssueType: "task", ParentID: "epic-b", Priority: 1},
		{ID: "epic-a", Title: "Epic A", IssueType: "epic", Priority: 2},
		{ID: "epic-b", Title: "Epic B", IssueType: "epic", Priority: 1},
		{ID: "epic-a.1", Title: "Epic A child", IssueType: "task", ParentID: "epic-a", Priority: 1},
	}

	got := SortHierarchically(input)

	// Epics sorted by priority: epic-b (P1) before epic-a (P2).
	// Each followed by children. Standalone last.
	wantOrder := []string{"epic-b", "epic-b.1", "epic-a", "epic-a.1", "task-standalone"}
	if len(got) != len(wantOrder) {
		t.Fatalf("expected %d beads, got %d: %+v", len(wantOrder), len(got), got)
	}
	for i, wantID := range wantOrder {
		if got[i].ID != wantID {
			t.Errorf("position %d: expected %s, got %s", i, wantID, got[i].ID)
		}
	}
}

func TestSortHierarchically_OrphanChildren(t *testing.T) {
	// Children whose parent epic isn't in the list should be treated as standalone.
	input := []Bead{
		{ID: "orphan-1", Title: "Orphan child", IssueType: "task", ParentID: "missing-epic", Priority: 1},
		{ID: "standalone", Title: "Standalone", IssueType: "bug", Priority: 2},
	}

	got := SortHierarchically(input)

	// Both treated as standalone, sorted by priority.
	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(got))
	}
	if got[0].ID != "orphan-1" {
		t.Errorf("expected orphan-1 first (priority 1), got %s", got[0].ID)
	}
	if got[1].ID != "standalone" {
		t.Errorf("expected standalone second (priority 2), got %s", got[1].ID)
	}
}

func TestSortHierarchically_AllStandalone(t *testing.T) {
	input := []Bead{
		{ID: "c", Title: "C", IssueType: "task", Priority: 3},
		{ID: "a", Title: "A", IssueType: "task", Priority: 1},
		{ID: "b", Title: "B", IssueType: "bug", Priority: 2},
	}

	got := SortHierarchically(input)

	wantOrder := []string{"a", "b", "c"}
	for i, wantID := range wantOrder {
		if got[i].ID != wantID {
			t.Errorf("position %d: expected %s, got %s", i, wantID, got[i].ID)
		}
	}
}

func TestSortHierarchically_ChildrenSortedByPriority(t *testing.T) {
	input := []Bead{
		{ID: "epic-1", Title: "Epic", IssueType: "epic", Priority: 1},
		{ID: "child-c", Title: "Low prio child", IssueType: "task", ParentID: "epic-1", Priority: 3},
		{ID: "child-a", Title: "High prio child", IssueType: "task", ParentID: "epic-1", Priority: 1},
		{ID: "child-b", Title: "Mid prio child", IssueType: "task", ParentID: "epic-1", Priority: 2},
	}

	got := SortHierarchically(input)

	wantOrder := []string{"epic-1", "child-a", "child-b", "child-c"}
	for i, wantID := range wantOrder {
		if got[i].ID != wantID {
			t.Errorf("position %d: expected %s, got %s", i, wantID, got[i].ID)
		}
	}
}

func TestParseBeads_ExtractsIssueTypeAndParentID(t *testing.T) {
	data := `[
		{"id": "epic-1", "title": "Epic", "status": "open", "issue_type": "epic", "dependencies": []},
		{"id": "child-1", "title": "Child", "status": "open", "issue_type": "task", "dependencies": [
			{"issue_id": "child-1", "depends_on_id": "epic-1", "type": "parent-child"}
		]}
	]`
	got, err := parseBeads([]byte(data))
	if err != nil {
		t.Fatalf("parseBeads returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(got))
	}
	if got[0].IssueType != "epic" {
		t.Errorf("expected epic issue_type, got %q", got[0].IssueType)
	}
	if got[0].ParentID != "" {
		t.Errorf("expected empty parent for epic, got %q", got[0].ParentID)
	}
	if got[1].IssueType != "task" {
		t.Errorf("expected task issue_type, got %q", got[1].IssueType)
	}
	if got[1].ParentID != "epic-1" {
		t.Errorf("expected parent epic-1, got %q", got[1].ParentID)
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
