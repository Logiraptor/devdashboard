package ralph

import (
	"encoding/json"
	"strings"
	"testing"
)

// mockBDShowFull returns a RunBDFunc that serves canned bd show --json output.
func mockBDShowFull(entry *bdShowFull) RunBDFunc {
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
		name    string
		substr  string
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
		{"blocking link", "--blocks test-abc.1"},
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
		ID:          "fetch-1",
		Title:       "Fetched bead",
		Description: "Full description from bd show.",
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
	// The bead ID should appear in: header, claim, close, and link instructions.
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
	// At minimum: header, claim, close, link = 4 occurrences
	if count < 4 {
		t.Errorf("expected bead ID to appear at least 4 times, got %d", count)
	}
}
