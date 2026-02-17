package project

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestPRInfo_JSONParsing_Valid tests parsing valid PR JSON with all fields.
func TestPRInfo_JSONParsing_Valid(t *testing.T) {
	jsonStr := `[
		{
			"number": 42,
			"title": "Fix bug in parser",
			"state": "OPEN",
			"headRefName": "fix/parser-bug",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 42 {
		t.Errorf("expected Number=42, got %d", pr.Number)
	}
	if pr.Title != "Fix bug in parser" {
		t.Errorf("expected Title='Fix bug in parser', got %q", pr.Title)
	}
	if pr.State != "OPEN" {
		t.Errorf("expected State='OPEN', got %q", pr.State)
	}
	if pr.HeadRefName != "fix/parser-bug" {
		t.Errorf("expected HeadRefName='fix/parser-bug', got %q", pr.HeadRefName)
	}
	if pr.MergedAt != nil {
		t.Errorf("expected MergedAt=nil, got %v", pr.MergedAt)
	}
}

// TestPRInfo_JSONParsing_MergedPR tests parsing a merged PR with mergedAt timestamp.
func TestPRInfo_JSONParsing_MergedPR(t *testing.T) {
	jsonStr := `[
		{
			"number": 100,
			"title": "Add feature",
			"state": "MERGED",
			"headRefName": "feature/new-thing",
			"mergedAt": "2024-01-15T10:30:00Z"
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 100 {
		t.Errorf("expected Number=100, got %d", pr.Number)
	}
	if pr.State != "MERGED" {
		t.Errorf("expected State='MERGED', got %q", pr.State)
	}
	if pr.MergedAt == nil {
		t.Fatal("expected MergedAt to be non-nil for merged PR")
	}
	expectedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !pr.MergedAt.Equal(expectedTime) {
		t.Errorf("expected MergedAt=%v, got %v", expectedTime, pr.MergedAt)
	}
}

// TestPRInfo_JSONParsing_MultiplePRs tests parsing multiple PRs in a single JSON array.
func TestPRInfo_JSONParsing_MultiplePRs(t *testing.T) {
	jsonStr := `[
		{
			"number": 1,
			"title": "First PR",
			"state": "OPEN",
			"headRefName": "branch-1",
			"mergedAt": null
		},
		{
			"number": 2,
			"title": "Second PR",
			"state": "CLOSED",
			"headRefName": "branch-2",
			"mergedAt": null
		},
		{
			"number": 3,
			"title": "Third PR",
			"state": "MERGED",
			"headRefName": "branch-3",
			"mergedAt": "2024-02-01T12:00:00Z"
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 3 {
		t.Fatalf("expected 3 PRs, got %d", len(prs))
	}

	// Verify first PR
	if prs[0].Number != 1 || prs[0].State != "OPEN" {
		t.Errorf("PR 1: expected Number=1, State=OPEN, got Number=%d, State=%s", prs[0].Number, prs[0].State)
	}

	// Verify second PR
	if prs[1].Number != 2 || prs[1].State != "CLOSED" {
		t.Errorf("PR 2: expected Number=2, State=CLOSED, got Number=%d, State=%s", prs[1].Number, prs[1].State)
	}

	// Verify third PR (merged)
	if prs[2].Number != 3 || prs[2].State != "MERGED" || prs[2].MergedAt == nil {
		t.Errorf("PR 3: expected Number=3, State=MERGED, MergedAt!=nil, got Number=%d, State=%s, MergedAt=%v",
			prs[2].Number, prs[2].State, prs[2].MergedAt)
	}
}

// TestPRInfo_JSONParsing_EmptyArray tests parsing an empty PR array.
func TestPRInfo_JSONParsing_EmptyArray(t *testing.T) {
	jsonStr := `[]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
}

// TestPRInfo_JSONParsing_MissingFields tests parsing PR JSON with missing optional fields.
// Missing fields should result in zero values.
func TestPRInfo_JSONParsing_MissingFields(t *testing.T) {
	jsonStr := `[
		{
			"number": 5
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 5 {
		t.Errorf("expected Number=5, got %d", pr.Number)
	}
	// Missing fields should be zero values
	if pr.Title != "" {
		t.Errorf("expected Title='' (zero value), got %q", pr.Title)
	}
	if pr.State != "" {
		t.Errorf("expected State='' (zero value), got %q", pr.State)
	}
	if pr.HeadRefName != "" {
		t.Errorf("expected HeadRefName='' (zero value), got %q", pr.HeadRefName)
	}
	if pr.MergedAt != nil {
		t.Errorf("expected MergedAt=nil (zero value), got %v", pr.MergedAt)
	}
}

// TestPRInfo_JSONParsing_EmptyStrings tests parsing PR JSON with empty string values.
func TestPRInfo_JSONParsing_EmptyStrings(t *testing.T) {
	jsonStr := `[
		{
			"number": 10,
			"title": "",
			"state": "",
			"headRefName": "",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if pr.Number != 10 {
		t.Errorf("expected Number=10, got %d", pr.Number)
	}
	if pr.Title != "" {
		t.Errorf("expected Title='', got %q", pr.Title)
	}
	if pr.State != "" {
		t.Errorf("expected State='', got %q", pr.State)
	}
	if pr.HeadRefName != "" {
		t.Errorf("expected HeadRefName='', got %q", pr.HeadRefName)
	}
}

// TestPRInfo_JSONParsing_InvalidJSON tests error handling for invalid JSON.
func TestPRInfo_JSONParsing_InvalidJSON(t *testing.T) {
	invalidJSON := `[{"number": invalid}]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(invalidJSON), &prs); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestPRInfo_JSONParsing_InvalidMergedAt tests parsing with invalid mergedAt timestamp.
func TestPRInfo_JSONParsing_InvalidMergedAt(t *testing.T) {
	jsonStr := `[
		{
			"number": 20,
			"title": "Test PR",
			"state": "OPEN",
			"headRefName": "test-branch",
			"mergedAt": "not-a-timestamp"
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err == nil {
		t.Error("expected error for invalid mergedAt timestamp, got nil")
	}
}

// TestPRInfo_JSONParsing_AllStates tests parsing PRs with all possible states.
func TestPRInfo_JSONParsing_AllStates(t *testing.T) {
	jsonStr := `[
		{"number": 1, "title": "Open", "state": "OPEN", "headRefName": "branch-1", "mergedAt": null},
		{"number": 2, "title": "Merged", "state": "MERGED", "headRefName": "branch-2", "mergedAt": "2024-01-01T00:00:00Z"},
		{"number": 3, "title": "Closed", "state": "CLOSED", "headRefName": "branch-3", "mergedAt": null}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 3 {
		t.Fatalf("expected 3 PRs, got %d", len(prs))
	}

	states := map[int]string{
		1: "OPEN",
		2: "MERGED",
		3: "CLOSED",
	}

	for _, pr := range prs {
		expectedState := states[pr.Number]
		if pr.State != expectedState {
			t.Errorf("PR #%d: expected State=%s, got %s", pr.Number, expectedState, pr.State)
		}
	}
}

// TestPRInfo_JSONParsing_RealWorldExample tests parsing a realistic GitHub PR list JSON response.
// This mimics the actual output format from `gh pr list --json number,title,state,headRefName,mergedAt`.
func TestPRInfo_JSONParsing_RealWorldExample(t *testing.T) {
	// This is a realistic example of what `gh pr list` returns
	jsonStr := `[
		{
			"number": 1234,
			"title": "feat: Add new authentication system",
			"state": "OPEN",
			"headRefName": "feat/auth-system",
			"mergedAt": null
		},
		{
			"number": 1233,
			"title": "fix: Resolve memory leak in parser",
			"state": "MERGED",
			"headRefName": "fix/memory-leak",
			"mergedAt": "2024-02-10T14:23:45Z"
		},
		{
			"number": 1232,
			"title": "docs: Update README",
			"state": "CLOSED",
			"headRefName": "docs/update-readme",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 3 {
		t.Fatalf("expected 3 PRs, got %d", len(prs))
	}

	// Verify all PRs have correct structure
	for i, pr := range prs {
		if pr.Number == 0 {
			t.Errorf("PR[%d]: Number should not be zero", i)
		}
		if pr.Title == "" {
			t.Errorf("PR[%d]: Title should not be empty", i)
		}
		if pr.State == "" {
			t.Errorf("PR[%d]: State should not be empty", i)
		}
		if pr.HeadRefName == "" {
			t.Errorf("PR[%d]: HeadRefName should not be empty", i)
		}
	}

	// Verify merged PR has MergedAt set
	mergedPR := prs[1] // Second PR is merged
	if mergedPR.State != "MERGED" {
		t.Fatalf("expected PR #1233 to be MERGED, got %s", mergedPR.State)
	}
	if mergedPR.MergedAt == nil {
		t.Error("expected MergedAt to be set for merged PR")
	}
}

// TestPRInfo_JSONParsing_MergedAtNullVsMissing tests that null mergedAt and missing mergedAt
// both result in nil pointer.
func TestPRInfo_JSONParsing_MergedAtNullVsMissing(t *testing.T) {
	jsonWithNull := `[
		{
			"number": 1,
			"title": "PR with null mergedAt",
			"state": "OPEN",
			"headRefName": "branch-1",
			"mergedAt": null
		}
	]`

	jsonWithoutField := `[
		{
			"number": 2,
			"title": "PR without mergedAt field",
			"state": "OPEN",
			"headRefName": "branch-2"
		}
	]`

	var prs1 []PRInfo
	if err := json.Unmarshal([]byte(jsonWithNull), &prs1); err != nil {
		t.Fatalf("json.Unmarshal (with null): %v", err)
	}

	var prs2 []PRInfo
	if err := json.Unmarshal([]byte(jsonWithoutField), &prs2); err != nil {
		t.Fatalf("json.Unmarshal (without field): %v", err)
	}

	if prs1[0].MergedAt != nil {
		t.Error("expected MergedAt=nil when explicitly set to null")
	}
	if prs2[0].MergedAt != nil {
		t.Error("expected MergedAt=nil when field is missing")
	}
}

// TestPRInfo_JSONParsing_Whitespace tests parsing JSON with various whitespace.
func TestPRInfo_JSONParsing_Whitespace(t *testing.T) {
	// JSON with extra whitespace and newlines
	jsonStr := `[
		{
			"number" : 99,
			"title" : "PR with whitespace",
			"state" : "OPEN",
			"headRefName" : "branch-99",
			"mergedAt" : null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 || prs[0].Number != 99 {
		t.Errorf("expected PR #99, got %d", prs[0].Number)
	}
}

// TestPRInfo_JSONParsing_LargeNumbers tests parsing PRs with large numbers.
func TestPRInfo_JSONParsing_LargeNumbers(t *testing.T) {
	jsonStr := `[
		{
			"number": 999999,
			"title": "Large PR number",
			"state": "OPEN",
			"headRefName": "branch-large",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 || prs[0].Number != 999999 {
		t.Errorf("expected Number=999999, got %d", prs[0].Number)
	}
}

// TestPRInfo_JSONParsing_SpecialCharacters tests parsing PR titles and branch names with special characters.
func TestPRInfo_JSONParsing_SpecialCharacters(t *testing.T) {
	jsonStr := `[
		{
			"number": 50,
			"title": "Fix: Handle \"quotes\" and 'apostrophes' & ampersands",
			"state": "OPEN",
			"headRefName": "fix/special-chars-123",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	expectedTitle := `Fix: Handle "quotes" and 'apostrophes' & ampersands`
	if pr.Title != expectedTitle {
		t.Errorf("expected Title=%q, got %q", expectedTitle, pr.Title)
	}
	if pr.HeadRefName != "fix/special-chars-123" {
		t.Errorf("expected HeadRefName='fix/special-chars-123', got %q", pr.HeadRefName)
	}
}

// TestPRInfo_JSONParsing_UnicodeCharacters tests parsing PR titles with Unicode characters.
func TestPRInfo_JSONParsing_UnicodeCharacters(t *testing.T) {
	jsonStr := `[
		{
			"number": 60,
			"title": "Fix: 修复中文问题 🐛",
			"state": "OPEN",
			"headRefName": "fix/unicode-测试",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	expectedTitle := "Fix: 修复中文问题 🐛"
	if pr.Title != expectedTitle {
		t.Errorf("expected Title=%q, got %q", expectedTitle, pr.Title)
	}
	if pr.HeadRefName != "fix/unicode-测试" {
		t.Errorf("expected HeadRefName='fix/unicode-测试', got %q", pr.HeadRefName)
	}
}

// TestPRInfo_JSONParsing_Marshaling tests that PRInfo can be marshaled back to JSON.
func TestPRInfo_JSONParsing_Marshaling(t *testing.T) {
	prs := []PRInfo{
		{
			Number:      42,
			Title:       "Test PR",
			State:       "OPEN",
			HeadRefName: "test-branch",
			MergedAt:    nil,
		},
		{
			Number:      43,
			Title:       "Merged PR",
			State:       "MERGED",
			HeadRefName: "merged-branch",
			MergedAt:    func() *time.Time { t := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC); return &t }(),
		},
	}

	jsonBytes, err := json.Marshal(prs)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Verify we can unmarshal it back
	var unmarshaled []PRInfo
	if err := json.Unmarshal(jsonBytes, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal (round-trip): %v", err)
	}

	if len(unmarshaled) != 2 {
		t.Fatalf("expected 2 PRs after round-trip, got %d", len(unmarshaled))
	}

	if unmarshaled[0].Number != 42 || unmarshaled[0].MergedAt != nil {
		t.Errorf("PR 1 round-trip failed: Number=%d, MergedAt=%v", unmarshaled[0].Number, unmarshaled[0].MergedAt)
	}
	if unmarshaled[1].Number != 43 || unmarshaled[1].MergedAt == nil {
		t.Errorf("PR 2 round-trip failed: Number=%d, MergedAt=%v", unmarshaled[1].Number, unmarshaled[1].MergedAt)
	}
}

// TestPRInfo_JSONParsing_ListPRsInRepoFormat tests that the JSON format matches
// what listPRsInRepo expects from `gh pr list --json number,title,state,headRefName,mergedAt`.
// This is an integration-style test that verifies the expected JSON structure.
func TestPRInfo_JSONParsing_ListPRsInRepoFormat(t *testing.T) {
	// This JSON structure matches the exact output format from:
	// gh pr list --json number,title,state,headRefName,mergedAt
	jsonStr := `[
		{
			"number": 100,
			"title": "Example PR",
			"state": "OPEN",
			"headRefName": "example-branch",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	// Verify all fields that listPRsInRepo expects are present
	if pr.Number == 0 {
		t.Error("Number field is required and should not be zero")
	}
	// Title, State, HeadRefName, and MergedAt are optional in the JSON
	// but should be parsed correctly when present
	_ = pr.Title
	_ = pr.State
	_ = pr.HeadRefName
	_ = pr.MergedAt
}

// TestPRInfo_JSONParsing_MalformedArray tests error handling for malformed JSON arrays.
func TestPRInfo_JSONParsing_MalformedArray(t *testing.T) {
	testCases := []struct {
		name      string
		jsonStr   string
		expectErr bool
	}{
		{
			name:      "missing closing bracket",
			jsonStr:   `[{"number": 1}`,
			expectErr: true,
		},
		{
			name:      "missing opening bracket",
			jsonStr:   `{"number": 1}]`,
			expectErr: true,
		},
		{
			name:      "trailing comma",
			jsonStr:   `[{"number": 1},]`,
			expectErr: true, // Go's json package does not allow trailing commas
		},
		{
			name:      "not an array",
			jsonStr:   `{"number": 1}`,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var prs []PRInfo
			err := json.Unmarshal([]byte(tc.jsonStr), &prs)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

// TestPRInfo_JSONParsing_TypeMismatch tests error handling for type mismatches.
func TestPRInfo_JSONParsing_TypeMismatch(t *testing.T) {
	testCases := []struct {
		name    string
		jsonStr string
	}{
		{
			name:    "number as string",
			jsonStr: `[{"number": "not-a-number"}]`,
		},
		{
			name:    "title as number",
			jsonStr: `[{"number": 1, "title": 123}]`,
		},
		{
			name:    "state as object",
			jsonStr: `[{"number": 1, "state": {}}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var prs []PRInfo
			err := json.Unmarshal([]byte(tc.jsonStr), &prs)
			if err == nil {
				t.Errorf("expected error for type mismatch in %s, got nil", tc.name)
			}
		})
	}
}

// TestPRInfo_JSONParsing_EdgeCaseMergedAt tests edge cases for mergedAt timestamps.
func TestPRInfo_JSONParsing_EdgeCaseMergedAt(t *testing.T) {
	testCases := []struct {
		name      string
		jsonStr   string
		expectErr bool
	}{
		{
			name:      "RFC3339 format",
			jsonStr:   `[{"number": 1, "mergedAt": "2024-01-01T12:00:00Z"}]`,
			expectErr: false,
		},
		{
			name:      "RFC3339 with nanoseconds",
			jsonStr:   `[{"number": 1, "mergedAt": "2024-01-01T12:00:00.123456789Z"}]`,
			expectErr: false,
		},
		{
			name:      "RFC3339 with timezone offset",
			jsonStr:   `[{"number": 1, "mergedAt": "2024-01-01T12:00:00+05:00"}]`,
			expectErr: false,
		},
		{
			name:      "empty string",
			jsonStr:   `[{"number": 1, "mergedAt": ""}]`,
			expectErr: true,
		},
		{
			name:      "invalid date format",
			jsonStr:   `[{"number": 1, "mergedAt": "2024-13-45"}]`,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var prs []PRInfo
			err := json.Unmarshal([]byte(tc.jsonStr), &prs)
			if tc.expectErr && err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error for %s: %v", tc.name, err)
			}
		})
	}
}

// TestPRInfo_JSONParsing_WhitespaceInStrings tests that whitespace in strings is preserved.
func TestPRInfo_JSONParsing_WhitespaceInStrings(t *testing.T) {
	jsonStr := `[
		{
			"number": 70,
			"title": "  PR with leading/trailing spaces  ",
			"state": "OPEN",
			"headRefName": "  branch-with-spaces  ",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	expectedTitle := "  PR with leading/trailing spaces  "
	if pr.Title != expectedTitle {
		t.Errorf("expected Title=%q, got %q", expectedTitle, pr.Title)
	}
	expectedBranch := "  branch-with-spaces  "
	if pr.HeadRefName != expectedBranch {
		t.Errorf("expected HeadRefName=%q, got %q", expectedBranch, pr.HeadRefName)
	}
}

// TestPRInfo_JSONParsing_NewlinesInStrings tests that newlines in strings are preserved.
func TestPRInfo_JSONParsing_NewlinesInStrings(t *testing.T) {
	jsonStr := `[
		{
			"number": 80,
			"title": "PR with\nnewlines\nin title",
			"state": "OPEN",
			"headRefName": "branch\nwith\nnewlines",
			"mergedAt": null
		}
	]`

	var prs []PRInfo
	if err := json.Unmarshal([]byte(jsonStr), &prs); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}

	pr := prs[0]
	if !strings.Contains(pr.Title, "\n") {
		t.Error("expected Title to contain newlines")
	}
	if !strings.Contains(pr.HeadRefName, "\n") {
		t.Error("expected HeadRefName to contain newlines")
	}
}
