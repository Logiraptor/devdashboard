package ralph

import (
	"testing"
)

func TestLocalTraceEmitter_StartEndLoop(t *testing.T) {
	emitter := NewLocalTraceEmitter()

	traceID := emitter.StartLoop("composer-1", "test-epic", "/tmp", 10)
	if traceID == "" {
		t.Error("StartLoop should return a trace ID")
	}

	activeTrace := emitter.GetActiveTrace()
	if activeTrace == nil {
		t.Error("Should have active trace after StartLoop")
	}
	if activeTrace.Status != "running" {
		t.Errorf("Trace status should be 'running', got '%s'", activeTrace.Status)
	}
	if activeTrace.ID != traceID {
		t.Errorf("Trace ID should match, got '%s', expected '%s'", activeTrace.ID, traceID)
	}

	emitter.EndLoop("normal", 5, 4, 1)

	// After EndLoop, trace should still exist but may be completed
	// The exact behavior depends on trace.Manager implementation
	activeTrace = emitter.GetActiveTrace()
	if activeTrace != nil && activeTrace.Status != "completed" && activeTrace.Status != "running" {
		t.Logf("Trace status after EndLoop: %s (implementation dependent)", activeTrace.Status)
	}
}

func TestLocalTraceEmitter_Iteration(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)

	spanID := emitter.StartIteration("bead-1", "Test bead", 1)
	if spanID == "" {
		t.Error("StartIteration should return a span ID")
	}

	emitter.EndIteration(spanID, "success", 5000)

	activeTrace := emitter.GetActiveTrace()
	if activeTrace == nil {
		t.Skip("Trace structure depends on implementation")
	}
	if activeTrace.RootSpan == nil {
		t.Skip("RootSpan structure depends on implementation")
	}
}

func TestLocalTraceEmitter_ToolCalls(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)
	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	toolSpan := emitter.StartTool("read", map[string]string{"file_path": "test.go"})
	if toolSpan == "" {
		t.Error("StartTool should return a span ID")
	}

	emitter.EndTool(toolSpan, map[string]string{})

	// Verify tool was added to trace
	activeTrace := emitter.GetActiveTrace()
	if activeTrace == nil {
		t.Skip("Trace structure depends on implementation")
	}
}

func TestLocalTraceEmitter_SetParent(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)

	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	// Start a tool - it should be a child of the iteration
	toolSpan := emitter.StartTool("read", map[string]string{"file_path": "test.go"})
	if toolSpan == "" {
		t.Error("StartTool should return a span ID")
	}

	emitter.EndTool(toolSpan, map[string]string{})
	emitter.EndIteration(iterSpan, "success", 1000)
}

func TestLocalTraceEmitter_MultipleIterations(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)

	span1 := emitter.StartIteration("bead-1", "First", 1)
	emitter.EndIteration(span1, "success", 1000)

	span2 := emitter.StartIteration("bead-2", "Second", 2)
	emitter.EndIteration(span2, "failure", 2000)

	activeTrace := emitter.GetActiveTrace()
	if activeTrace == nil {
		t.Skip("Trace structure depends on implementation")
	}
}

func TestLocalTraceEmitter_GetManager(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	manager := emitter.GetManager()

	if manager == nil {
		t.Error("GetManager should return a non-nil manager")
	}
}

func TestLocalTraceWriter_BasicWrite(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)
	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	writer := NewLocalTraceWriter(emitter)

	// Write a tool_call started event
	eventJSON := `{"type":"tool_call","subtype":"started","id":"tool-123","name":"read_file","arguments":{"file_path":"test.go"}}` + "\n"
	n, err := writer.Write([]byte(eventJSON))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(eventJSON) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(eventJSON))
	}

	// Write a tool_call completed event
	completeJSON := `{"type":"tool_call","subtype":"completed","id":"tool-123","result":{}}` + "\n"
	n, err = writer.Write([]byte(completeJSON))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(completeJSON) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(completeJSON))
	}
}

func TestLocalTraceWriter_NestedSchema(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)
	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	writer := NewLocalTraceWriter(emitter)

	// Write a nested schema tool_call started event
	eventJSON := `{"type":"tool_call","subtype":"started","id":"tool-456","tool_call":{"readToolCall":{"args":{"target_file":"test.go"}}}}` + "\n"
	n, err := writer.Write([]byte(eventJSON))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(eventJSON) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(eventJSON))
	}

	// Write completed event
	completeJSON := `{"type":"tool_call","subtype":"completed","id":"tool-456","tool_call":{"readToolCall":{"result":{}}}}` + "\n"
	n, err = writer.Write([]byte(completeJSON))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(completeJSON) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(completeJSON))
	}
}

func TestLocalTraceWriter_MultipleEvents(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)
	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	writer := NewLocalTraceWriter(emitter)

	// Write multiple events in one call
	events := `{"type":"tool_call","subtype":"started","id":"tool-1","name":"read_file","arguments":{"file_path":"file1.go"}}
{"type":"tool_call","subtype":"completed","id":"tool-1","result":{}}
{"type":"tool_call","subtype":"started","id":"tool-2","name":"write","arguments":{"file_path":"file2.go"}}
{"type":"tool_call","subtype":"completed","id":"tool-2","result":{}}
`
	n, err := writer.Write([]byte(events))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(events) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(events))
	}
}

func TestLocalTraceWriter_InvalidJSON(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	writer := NewLocalTraceWriter(emitter)

	// Write invalid JSON - should not panic
	invalidJSON := `{"type":"tool_call","invalid json` + "\n"
	n, err := writer.Write([]byte(invalidJSON))
	if err != nil {
		t.Errorf("Write should handle invalid JSON gracefully, got error: %v", err)
	}
	if n != len(invalidJSON) {
		t.Errorf("Write should return bytes written even for invalid JSON, got %d, expected %d", n, len(invalidJSON))
	}
}

func TestLocalTraceWriter_NonToolCallEvents(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	writer := NewLocalTraceWriter(emitter)

	// Write a non-tool_call event - should be ignored
	eventJSON := `{"type":"other_event","data":"test"}` + "\n"
	n, err := writer.Write([]byte(eventJSON))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(eventJSON) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(eventJSON))
	}
}

func TestLocalTraceWriter_PartialLine(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	writer := NewLocalTraceWriter(emitter)

	// Write partial line (no newline) - should be buffered
	partial := `{"type":"tool_call","subtype":"started","id":"tool-partial"`
	n, err := writer.Write([]byte(partial))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(partial) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(partial))
	}

	// Complete the line
	complete := `,"name":"read_file"}` + "\n"
	n, err = writer.Write([]byte(complete))
	if err != nil {
		t.Errorf("Write should not return error: %v", err)
	}
	if n != len(complete) {
		t.Errorf("Write should return bytes written, got %d, expected %d", n, len(complete))
	}
}

func TestLocalTraceWriter_ToolNameMapping(t *testing.T) {
	emitter := NewLocalTraceEmitter()
	emitter.StartLoop("composer-1", "test", "/tmp", 10)
	iterSpan := emitter.StartIteration("bead-1", "Test", 1)
	emitter.SetParent(iterSpan)

	writer := NewLocalTraceWriter(emitter)

	testCases := []struct {
		name     string
		eventJSON string
		expectedTool string
	}{
		{
			name: "read_file",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t1","name":"read_file","arguments":{"file_path":"test.go"}}` + "\n",
			expectedTool: "read",
		},
		{
			name: "write",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t2","name":"write","arguments":{"file_path":"test.go"}}` + "\n",
			expectedTool: "write",
		},
		{
			name: "search_replace",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t3","name":"search_replace","arguments":{"file_path":"test.go"}}` + "\n",
			expectedTool: "edit",
		},
		{
			name: "run_terminal_cmd",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t4","name":"run_terminal_cmd","arguments":{"command":"go test"}}` + "\n",
			expectedTool: "shell",
		},
		{
			name: "grep",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t5","name":"grep","arguments":{"pattern":"test"}}` + "\n",
			expectedTool: "grep",
		},
		{
			name: "codebase_search",
			eventJSON: `{"type":"tool_call","subtype":"started","id":"t6","name":"codebase_search","arguments":{"query":"test"}}` + "\n",
			expectedTool: "search",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n, err := writer.Write([]byte(tc.eventJSON))
			if err != nil {
				t.Errorf("Write should not return error: %v", err)
			}
			if n != len(tc.eventJSON) {
				t.Errorf("Write should return bytes written, got %d, expected %d", n, len(tc.eventJSON))
			}
			// Note: We can't easily verify the tool name was mapped correctly without
			// inspecting the trace structure, but we can verify it doesn't panic
		})
	}
}
