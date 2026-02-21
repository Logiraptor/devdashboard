package ralph

import (
	"encoding/json"
	"strings"
	"testing"

	"devdeploy/internal/bd"
)

// mockBDShowFull returns a bd.Runner that serves canned bd show --json output.
func mockBDShowFull(entry *bdShowFull) bd.Runner {
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

func TestRenderPrompt_ContainsSkillAndContext(t *testing.T) {
	data := &PromptData{
		ID:          "test-abc.1",
		Title:       "Implement widget factory",
		Description: "Build the widget factory that produces widgets.\n\n## Requirements\n- Fast\n- Reliable",
		IssueType:   "task",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		name   string
		substr string
	}{
		{"skill invocation", "/work-bead"},
		{"bead ID", "Bead ID: test-abc.1"},
		{"title", "# Implement widget factory"},
		{"description", "Build the widget factory"},
	}

	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("prompt missing %s (expected substring %q)\n\nFull prompt:\n%s", c.name, c.substr, got)
		}
	}
}

func TestRenderPrompt_PreservesSpecialCharacters(t *testing.T) {
	data := &PromptData{
		ID:          "x-1",
		Title:       "Handle <script> tags & ampersands",
		Description: "Description with <html> & \"quotes\"",
		IssueType:   "task",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "<script>") {
		t.Error("should not escape HTML; <script> was escaped")
	}
	if !strings.Contains(got, `"quotes"`) {
		t.Error("should not escape double quotes")
	}
}

func TestRenderPrompt_EmptyDescription(t *testing.T) {
	data := &PromptData{
		ID:          "empty-1",
		Title:       "No description bead",
		Description: "",
		IssueType:   "task",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
		IssueType: "task",
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
		IssueType: "task",
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
	if got.IssueType != "task" {
		t.Errorf("IssueType = %q, want %q", got.IssueType, "task")
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

func TestRenderPrompt_EpicUsesEpicSkill(t *testing.T) {
	data := &PromptData{
		ID:          "epic-1",
		Title:       "Epic Title",
		Description: "Epic description",
		IssueType:   "epic",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "/work-epic") {
		t.Error("epic prompt should invoke /work-epic skill")
	}
	if !strings.Contains(got, "Bead ID: epic-1") {
		t.Error("epic prompt should contain bead ID")
	}
	if !strings.Contains(got, "# Epic Title") {
		t.Error("epic prompt should contain title")
	}
}

func TestRenderPrompt_TaskUsesBeadSkill(t *testing.T) {
	data := &PromptData{
		ID:          "task-1",
		Title:       "Task Title",
		Description: "Task description",
		IssueType:   "task",
	}

	got, err := RenderPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "/work-bead") {
		t.Error("task prompt should invoke /work-bead skill")
	}
	if strings.Contains(got, "/work-epic") {
		t.Error("task prompt should not invoke /work-epic skill")
	}
}

func TestRenderVerifyPrompt_UsesVerifySkill(t *testing.T) {
	data := &PromptData{
		ID:          "verify-1",
		Title:       "Verify Title",
		Description: "Verify description",
		IssueType:   "task",
	}

	got, err := RenderVerifyPrompt(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(got, "/verify-bead") {
		t.Error("verify prompt should invoke /verify-bead skill")
	}
	if !strings.Contains(got, "Bead ID: verify-1") {
		t.Error("verify prompt should contain bead ID")
	}
}
