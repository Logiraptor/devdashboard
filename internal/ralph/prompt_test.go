package ralph

import (
	"encoding/json"
	"strings"
	"testing"
)

// mockBDShowFull returns a BDRunner that serves canned bd show --json output.
func mockBDShowFull(entry *bdShowFull) BDRunner {
	return func(dir string, args ...string) ([]byte, error) {
		if entry == nil {
			return nil, errBDNotFound
		}
		data, err := json.Marshal([]bdShowFull{*entry})
		return data, err
	}
}

var errBDNotFound = &mockError{"bd: bead not found"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func TestRenderPrompt_ContainsAllSections(t *testing.T) {
	data := &PromptData{
		ID:          "test-abc.1",
		Title:       "Implement widget factory",
		Description: "Build the widget factory that produces widgets.\n\n## Requirements\n- Fast\n- Reliable",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all required sections are present.
	checks := []struct {
		name   string
		substr string
	}{
		{"bead ID header", "bead test-abc.1"},
		{"title", "# Implement widget factory"},
		{"description", "Build the widget factory"},
		{"claim instruction", "bd update test-abc.1 --status in_progress"},
		{"close instruction", "bd close test-abc.1"},
		{"cursor rules", ".cursor/rules/"},
		{"AGENTS.md", "AGENTS.md"},
		{"push instruction", "git push"},
		{"question protocol", "needs-human"},
		{"blocking dependency", "bd dep add test-abc.1"},
		{"parent relationship", "--parent test-abc.1"},
		{"stop instruction", "Stop working on this bead"},
	}

	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("prompt missing %s (expected substring %q)\n\nFull prompt:\n%s", c.name, c.substr, got)
		}
	}
}

func TestRenderPrompt_EscapesNothing(t *testing.T) {
	// Ensure template does not HTML-escape content (text/template, not html/template).
	data := &PromptData{
		ID:          "x-1",
		Title:       "Handle <script> tags & ampersands",
		Description: "Description with <html> & \"quotes\"",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "<script>") {
		t.Error("text/template should not HTML-escape; <script> was escaped")
	}
	if !strings.Contains(got, `"quotes"`) {
		t.Error("text/template should not escape double quotes")
	}
}

func TestRenderPrompt_EmptyDescription(t *testing.T) {
	data := &PromptData{
		ID:          "empty-1",
		Title:       "No description bead",
		Description: "",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still render without error, with the ID and title present.
	if !strings.Contains(got, "empty-1") {
		t.Error("expected bead ID in rendered prompt")
	}
	if !strings.Contains(got, "No description bead") {
		t.Error("expected title in rendered prompt")
	}
}

func TestRenderPrompt_MultilineDescription(t *testing.T) {
	data := &PromptData{
		ID:    "multi-1",
		Title: "Multi-line work",
		Description: `First paragraph.

## Section A
- Item 1
- Item 2

## Section B
Code block:
` + "```go" + `
func main() {}
` + "```",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "## Section A") {
		t.Error("expected markdown sections preserved in prompt")
	}
	if !strings.Contains(got, "func main() {}") {
		t.Error("expected code block content preserved in prompt")
	}
}

func TestFetchPromptData_Success(t *testing.T) {
	runner := mockBDShowFull(&bdShowFull{
		bdShowBase: bdShowBase{
			ID:          "fetch-1",
			Title:       "Fetched bead",
			Description: "Full description from bd show.",
		},
	})

	got, err := FetchPromptData(runner, "/fake", "fetch-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "fetch-1" {
		t.Errorf("ID = %q, want %q", got.ID, "fetch-1")
	}
	if got.Title != "Fetched bead" {
		t.Errorf("Title = %q, want %q", got.Title, "Fetched bead")
	}
	if got.Description != "Full description from bd show." {
		t.Errorf("Description = %q, want %q", got.Description, "Full description from bd show.")
	}
}

func TestFetchPromptData_BDError(t *testing.T) {
	runner := mockBDShowFull(nil) // returns error

	_, err := FetchPromptData(runner, "/fake", "missing-1")
	if err == nil {
		t.Fatal("expected error when bd fails, got nil")
	}
}

func TestFetchPromptData_InvalidJSON(t *testing.T) {
	runner := func(dir string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	_, err := FetchPromptData(runner, "/fake", "bad-1")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestFetchPromptData_EmptyArray(t *testing.T) {
	runner := func(dir string, args ...string) ([]byte, error) {
		return []byte("[]"), nil
	}

	_, err := FetchPromptData(runner, "/fake", "empty-1")
	if err == nil {
		t.Fatal("expected error for empty result, got nil")
	}
}

func TestRenderPrompt_IDAppearsMultipleTimes(t *testing.T) {
	// The bead ID should appear in: header, claim, close, parent, and dep add instructions.
	data := &PromptData{
		ID:          "repeat-42",
		Title:       "Test",
		Description: "Desc",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := strings.Count(got, "repeat-42")
	// At minimum: header, claim, close, parent, dep add = 5 occurrences
	if count < 5 {
		t.Errorf("expected bead ID to appear at least 5 times, got %d", count)
	}
}

// mockBDShowWithDependents returns a BDRunner that serves canned bd show --json output with dependents.
func mockBDShowWithDependents(entry *bdShowWithDependents) BDRunner {
	return func(dir string, args ...string) ([]byte, error) {
		if entry == nil {
			return nil, errBDNotFound
		}
		data, err := json.Marshal([]bdShowWithDependents{*entry})
		return data, err
	}
}

func TestFetchVerificationPromptData_Success(t *testing.T) {
	runner := mockBDShowWithDependents(&bdShowWithDependents{
		bdShowBase: bdShowBase{
			ID:          "epic-1",
			Title:       "Test Epic",
			Description: "Epic description",
		},
		Dependents: []bdShowDependent{
			{
				ID:     "epic-1.1",
				Title:  "Child 1",
				Status: "closed",
				Labels: []string{},
			},
			{
				ID:     "epic-1.2",
				Title:  "Child 2",
				Status: "open",
				Labels: []string{"needs-human"},
			},
			{
				ID:     "epic-1.3",
				Title:  "Child 3",
				Status: "in_progress",
				Labels: []string{},
			},
		},
	})

	got, err := FetchVerificationPromptData(runner, "/fake", "epic-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID != "epic-1" {
		t.Errorf("ID = %q, want %q", got.ID, "epic-1")
	}
	if got.Title != "Test Epic" {
		t.Errorf("Title = %q, want %q", got.Title, "Test Epic")
	}
	if got.Description != "Epic description" {
		t.Errorf("Description = %q, want %q", got.Description, "Epic description")
	}
	if len(got.Children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(got.Children))
	}

	// Check first child (closed = success)
	if got.Children[0].ID != "epic-1.1" {
		t.Errorf("Children[0].ID = %q, want %q", got.Children[0].ID, "epic-1.1")
	}
	if got.Children[0].Outcome != OutcomeSuccess {
		t.Errorf("Children[0].Outcome = %v, want %v", got.Children[0].Outcome, OutcomeSuccess)
	}

	// Check second child (needs-human = question)
	if got.Children[1].ID != "epic-1.2" {
		t.Errorf("Children[1].ID = %q, want %q", got.Children[1].ID, "epic-1.2")
	}
	if got.Children[1].Outcome != OutcomeQuestion {
		t.Errorf("Children[1].Outcome = %v, want %v", got.Children[1].Outcome, OutcomeQuestion)
	}

	// Check third child (in_progress without needs-human = failure)
	if got.Children[2].ID != "epic-1.3" {
		t.Errorf("Children[2].ID = %q, want %q", got.Children[2].ID, "epic-1.3")
	}
	if got.Children[2].Outcome != OutcomeFailure {
		t.Errorf("Children[2].Outcome = %v, want %v", got.Children[2].Outcome, OutcomeFailure)
	}
}

func TestFetchVerificationPromptData_NoChildren(t *testing.T) {
	runner := mockBDShowWithDependents(&bdShowWithDependents{
		bdShowBase: bdShowBase{
			ID:          "epic-empty",
			Title:       "Empty Epic",
			Description: "No children",
		},
		Dependents: []bdShowDependent{},
	})

	got, err := FetchVerificationPromptData(runner, "/fake", "epic-empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(got.Children) != 0 {
		t.Errorf("expected 0 children, got %d", len(got.Children))
	}
}

func TestFetchVerificationPromptData_BDError(t *testing.T) {
	runner := mockBDShowWithDependents(nil) // returns error

	_, err := FetchVerificationPromptData(runner, "/fake", "missing-1")
	if err == nil {
		t.Fatal("expected error when bd fails, got nil")
	}
}

func TestRenderVerificationPrompt_ContainsAllSections(t *testing.T) {
	data := &VerificationPromptData{
		ID:          "epic-test",
		Title:       "Test Epic",
		Description: "Epic description",
		Children: []ChildSummary{
			{
				ID:      "epic-test.1",
				Title:   "Child 1",
				Status:  "closed",
				Outcome: OutcomeSuccess,
			},
			{
				ID:      "epic-test.2",
				Title:   "Child 2",
				Status:  "open",
				Outcome: OutcomeFailure,
			},
		},
	}

	got, err := RenderVerificationPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all required sections are present.
	checks := []struct {
		name   string
		substr string
	}{
		{"epic ID header", "bead epic-test"},
		{"title", "# Test Epic"},
		{"description", "Epic description"},
		{"children section", "Children Summary"},
		{"child ID", "epic-test.1"},
		{"child title", "Child 1"},
		{"status", "Status:"},
		{"outcome", "Outcome:"},
		{"review instruction", "Review completed work"},
		{"unblock instruction", "Unblock failures"},
		{"quality checks", "Run quality checks"},
		{"close instruction", "bd close epic-test"},
		{"push instruction", "git push"},
		{"main repo note", "main repository"},
	}

	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("prompt missing %s (expected substring %q)\n\nFull prompt:\n%s", c.name, c.substr, got)
		}
	}
}

func TestRenderVerificationPrompt_NoChildren(t *testing.T) {
	data := &VerificationPromptData{
		ID:          "epic-empty",
		Title:       "Empty Epic",
		Description: "No children",
		Children:    []ChildSummary{},
	}

	got, err := RenderVerificationPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "No child beads found") {
		t.Error("expected 'No child beads found' message when there are no children")
	}
}

func TestInferOutcome(t *testing.T) {
	tests := []struct {
		name   string
		status string
		labels []string
		want   Outcome
	}{
		{"closed", "closed", []string{}, OutcomeSuccess},
		{"open with needs-human", "open", []string{"needs-human"}, OutcomeQuestion},
		{"in_progress with needs-human", "in_progress", []string{"needs-human"}, OutcomeQuestion},
		{"open without needs-human", "open", []string{}, OutcomeFailure},
		{"in_progress without needs-human", "in_progress", []string{}, OutcomeFailure},
		{"open with other labels", "open", []string{"bug", "urgent"}, OutcomeFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferOutcome(tt.status, tt.labels)
			if got != tt.want {
				t.Errorf("inferOutcome(%q, %v) = %v, want %v", tt.status, tt.labels, got, tt.want)
			}
		})
	}
}
