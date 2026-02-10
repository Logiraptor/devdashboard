package ralph

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLogFormatter_NewSchema_SemSearchToolCall(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"semSearchToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"query": "How does authentication work?",
				},
				"result": map[string]interface{}{
					"success": map[string]interface{}{
						"results": "<huge string that should not appear>",
					},
				},
			},
		},
	}

	output := f.formatEvent(event)
	want := "[search] How does authentication work?"
	if output != want {
		t.Errorf("output = %q, want %q", output, want)
	}

	// Counter should be incremented
	summary := f.Summary()
	if !strings.Contains(summary, "searched 1") {
		t.Errorf("summary should show searched 1, got %q", summary)
	}
}

func TestLogFormatter_NewSchema_SemSearchToolCall_WithLongQuery(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"semSearchToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"query": strings.Repeat("a", 100), // Long query
				},
			},
		},
	}

	output := f.formatEvent(event)
	// Should be skipped (too noisy), but if we were to show it, it should be truncated
	if output != "" {
		if len(output) > 100 {
			t.Errorf("output should be truncated, got length %d: %q", len(output), output)
		}
	}
}

func TestLogFormatter_NewSchema_EditToolCall(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"editToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"path": "internal/ralph/logfmt.go",
				},
			},
		},
	}

	output := f.formatEvent(event)
	want := "[edit] internal/ralph/logfmt.go"
	if output != want {
		t.Errorf("output = %q, want %q", output, want)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "edited 1") {
		t.Errorf("summary should show edited 1, got %q", summary)
	}
}

func TestLogFormatter_NewSchema_ReadToolCall(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"readToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"target_file": "internal/ralph/logfmt.go",
				},
			},
		},
	}

	output := f.formatEvent(event)
	if output != "" {
		t.Errorf("read should be skipped (too noisy), got %q", output)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "read 1") {
		t.Errorf("summary should show read 1, got %q", summary)
	}
}

func TestLogFormatter_NewSchema_GrepToolCall(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"grepToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"pattern": "func.*format",
				},
			},
		},
	}

	output := f.formatEvent(event)
	if output != "" {
		t.Errorf("grep should be skipped (too noisy), got %q", output)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "searched 1") {
		t.Errorf("summary should show searched 1, got %q", summary)
	}
}

func TestLogFormatter_NewSchema_ShellToolCall(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"shellToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"command": "go test ./internal/ralph/...",
				},
			},
		},
	}

	output := f.formatEvent(event)
	want := "[shell] go test ./internal/ralph/..."
	if output != want {
		t.Errorf("output = %q, want %q", output, want)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "commands 1") {
		t.Errorf("summary should show commands 1, got %q", summary)
	}
}

func TestLogFormatter_NewSchema_ShellToolCall_LongCommand(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	longCmd := strings.Repeat("a", 100)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"shellToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"command": longCmd,
				},
			},
		},
	}

	output := f.formatEvent(event)
	if len(output) > 100 {
		t.Errorf("output should be truncated, got length %d: %q", len(output), output)
	}
	if !strings.HasSuffix(output, "...") {
		t.Errorf("truncated output should end with '...', got %q", output)
	}
}

func TestLogFormatter_NewSchema_ResultsIgnored(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	hugeResult := strings.Repeat("x", 10000)
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"semSearchToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"query": "test query",
				},
				"result": map[string]interface{}{
					"success": map[string]interface{}{
						"results": hugeResult,
					},
				},
			},
		},
	}

	// Serialize to JSON to ensure result is present
	jsonBytes, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	// Parse back and format
	var parsedEvent map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsedEvent); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	output := f.formatEvent(parsedEvent)
	// Result should never appear in output
	if strings.Contains(output, hugeResult) {
		t.Errorf("output should not contain result contents, got %q", output)
	}
	if strings.Contains(output, "x") {
		t.Errorf("output should not contain any part of result, got %q", output)
	}
}

func TestLogFormatter_OldSchema_BackwardsCompatibility(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)
	event := map[string]interface{}{
		"type": "tool_call",
		"name": "read_file",
		"arguments": map[string]interface{}{
			"file_path": "test.go",
		},
	}

	output := f.formatEvent(event)
	if output != "" {
		t.Errorf("read should be skipped (too noisy), got %q", output)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "read 1") {
		t.Errorf("summary should show read 1, got %q", summary)
	}
}

func TestLogFormatter_Write_NewSchema(t *testing.T) {
	var buf bytes.Buffer
	f := NewLogFormatter(&buf, false)

	// Write a semantic search event
	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"semSearchToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"query": "test query",
				},
			},
		},
	}
	jsonBytes, _ := json.Marshal(event)
	jsonBytes = append(jsonBytes, '\n')
	f.Write(jsonBytes)

	// Should output the search line
	output := buf.String()
	want := "[search] test query\n"
	if output != want {
		t.Errorf("Write output = %q, want %q", output, want)
	}
}

func TestLogFormatter_Write_EditEvent(t *testing.T) {
	var buf bytes.Buffer
	f := NewLogFormatter(&buf, false)

	event := map[string]interface{}{
		"type": "tool_call",
		"tool_call": map[string]interface{}{
			"editToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"path": "test.go",
				},
			},
		},
	}
	jsonBytes, _ := json.Marshal(event)
	jsonBytes = append(jsonBytes, '\n')
	f.Write(jsonBytes)

	output := buf.String()
	want := "[edit] test.go\n"
	if output != want {
		t.Errorf("Write output = %q, want %q", output, want)
	}
}

func TestLogFormatter_Summary_CountsAllToolTypes(t *testing.T) {
	f := NewLogFormatter(&bytes.Buffer{}, false)

	// Add one of each tool type
	tools := []map[string]interface{}{
		{
			"type": "tool_call",
			"tool_call": map[string]interface{}{
				"readToolCall": map[string]interface{}{
					"args": map[string]interface{}{"target_file": "a.go"},
				},
			},
		},
		{
			"type": "tool_call",
			"tool_call": map[string]interface{}{
				"semSearchToolCall": map[string]interface{}{
					"args": map[string]interface{}{"query": "q1"},
				},
			},
		},
		{
			"type": "tool_call",
			"tool_call": map[string]interface{}{
				"grepToolCall": map[string]interface{}{
					"args": map[string]interface{}{"pattern": "p1"},
				},
			},
		},
		{
			"type": "tool_call",
			"tool_call": map[string]interface{}{
				"editToolCall": map[string]interface{}{
					"args": map[string]interface{}{"path": "b.go"},
				},
			},
		},
		{
			"type": "tool_call",
			"tool_call": map[string]interface{}{
				"shellToolCall": map[string]interface{}{
					"args": map[string]interface{}{"command": "ls"},
				},
			},
		},
	}

	for _, tool := range tools {
		f.formatEvent(tool)
	}

	summary := f.Summary()
	if !strings.Contains(summary, "read 1") {
		t.Errorf("summary should contain 'read 1', got %q", summary)
	}
	if !strings.Contains(summary, "searched 2") {
		t.Errorf("summary should contain 'searched 2' (semSearch + grep), got %q", summary)
	}
	if !strings.Contains(summary, "edited 1") {
		t.Errorf("summary should contain 'edited 1', got %q", summary)
	}
	if !strings.Contains(summary, "commands 1") {
		t.Errorf("summary should contain 'commands 1', got %q", summary)
	}
}

func TestLogFormatter_ToolCallSubtype(t *testing.T) {
	var buf bytes.Buffer
	f := NewLogFormatter(&buf, false)

	// Process a "started" tool_call event - should be counted
	started := map[string]interface{}{
		"type":    "tool_call",
		"subtype": "started",
		"tool_call": map[string]interface{}{
			"readToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"target_file": "test.txt",
				},
			},
		},
	}
	f.formatEvent(started)

	// Process a "completed" tool_call event - should be ignored (not counted again)
	completed := map[string]interface{}{
		"type":    "tool_call",
		"subtype": "completed",
		"tool_call": map[string]interface{}{
			"readToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"target_file": "test.txt",
				},
				"result": map[string]interface{}{
					"success": map[string]interface{}{},
				},
			},
		},
	}
	f.formatEvent(completed)

	// Should only count once, not twice
	summary := f.Summary()
	if !strings.Contains(summary, "read 1") {
		t.Errorf("expected readsCount=1, got summary: %q", summary)
	}
	if strings.Contains(summary, "read 2") {
		t.Errorf("should not count completed events, got summary: %q", summary)
	}
}
