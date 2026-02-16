package tui

import (
	"testing"

	"devdeploy/internal/trace"
)

// TestLocalTraceEmitter_ToolSpans verifies that StartTool/EndTool creates
// proper span tree structure in the trace manager.
func TestLocalTraceEmitter_ToolSpans(t *testing.T) {
	emitter := NewLocalTraceEmitter()

	// Start a loop to establish trace context
	traceID := emitter.StartLoop("composer-1", "test-epic", "/tmp/test", 10)
	if traceID == "" {
		t.Fatal("StartLoop should return a trace ID")
	}

	// Start an iteration to establish parent context
	iterSpanID := emitter.StartIteration("bead-123", "Test Bead", 1)
	if iterSpanID == "" {
		t.Fatal("StartIteration should return a span ID")
	}

	// Start a tool call
	toolAttrs := map[string]string{
		"file_path": "test.go",
	}
	toolSpanID := emitter.StartTool("read", toolAttrs)
	if toolSpanID == "" {
		t.Fatal("StartTool should return a span ID")
	}

	// Verify the trace manager has the tool span
	manager := emitter.Manager()
	activeTrace := manager.ActiveTrace()
	if activeTrace == nil {
		t.Fatal("GetActiveTrace should return a trace")
	}

	// Find the iteration span
	var iterSpan *trace.Span
	if activeTrace.RootSpan != nil {
		for _, child := range activeTrace.RootSpan.Children {
			if child.SpanID == iterSpanID {
				iterSpan = child
				break
			}
		}
	}

	if iterSpan == nil {
		t.Fatal("iteration span should exist in trace")
	}

	// Verify tool span is a child of the iteration span
	var toolSpan *trace.Span
	for _, child := range iterSpan.Children {
		if child.SpanID == toolSpanID {
			toolSpan = child
			break
		}
	}

	if toolSpan == nil {
		t.Fatal("tool span should be a child of iteration span")
	}

	if toolSpan.Name != "read" {
		t.Errorf("tool span name = %q, want %q", toolSpan.Name, "read")
	}

	if toolSpan.Attributes["file_path"] != "test.go" {
		t.Errorf("tool span file_path = %q, want %q", toolSpan.Attributes["file_path"], "test.go")
	}

	// End the tool call
	endAttrs := map[string]string{
		"duration_ms": "100",
	}
	emitter.EndTool(toolSpanID, endAttrs)

	// Verify the tool span still exists (EndTool doesn't remove it, just updates it)
	activeTrace = manager.ActiveTrace()
	if activeTrace == nil {
		t.Fatal("ActiveTrace should still return a trace after EndTool")
	}

	// Find the tool span again to verify it's still there
	iterSpan = nil
	if activeTrace.RootSpan != nil {
		for _, child := range activeTrace.RootSpan.Children {
			if child.SpanID == iterSpanID {
				iterSpan = child
				break
			}
		}
	}

	if iterSpan == nil {
		t.Fatal("iteration span should still exist")
	}

	toolSpan = nil
	for _, child := range iterSpan.Children {
		if child.SpanID == toolSpanID {
			toolSpan = child
			break
		}
	}

	if toolSpan == nil {
		t.Fatal("tool span should still exist after EndTool")
	}

	// Verify end attributes were added
	if toolSpan.Attributes["duration_ms"] != "100" {
		t.Errorf("tool span duration_ms = %q, want %q", toolSpan.Attributes["duration_ms"], "100")
	}
}

// TestLocalTraceEmitter_ToolSpanTreeStructure verifies that multiple tool calls
// create a proper tree structure with correct parent-child relationships.
func TestLocalTraceEmitter_ToolSpanTreeStructure(t *testing.T) {
	emitter := NewLocalTraceEmitter()

	// Start loop and iteration
	emitter.StartLoop("composer-1", "test-epic", "/tmp/test", 10)
	iterSpanID := emitter.StartIteration("bead-123", "Test Bead", 1)

	// Start multiple tool calls
	tool1SpanID := emitter.StartTool("read", map[string]string{"file_path": "file1.go"})
	tool2SpanID := emitter.StartTool("write", map[string]string{"file_path": "file2.go"})
	tool3SpanID := emitter.StartTool("shell", map[string]string{"command": "go test"})

	// End tools
	emitter.EndTool(tool1SpanID, map[string]string{"duration_ms": "50"})
	emitter.EndTool(tool2SpanID, map[string]string{"duration_ms": "75"})
	emitter.EndTool(tool3SpanID, map[string]string{"duration_ms": "200"})

	// Verify all tools are children of the iteration span
	manager := emitter.Manager()
	activeTrace := manager.ActiveTrace()
	if activeTrace == nil {
		t.Fatal("GetActiveTrace should return a trace")
	}

	var iterSpan *trace.Span
	if activeTrace.RootSpan != nil {
		for _, child := range activeTrace.RootSpan.Children {
			if child.SpanID == iterSpanID {
				iterSpan = child
				break
			}
		}
	}

	if iterSpan == nil {
		t.Fatal("iteration span should exist")
	}

	if len(iterSpan.Children) != 3 {
		t.Fatalf("iteration span should have 3 tool children, got %d", len(iterSpan.Children))
	}

	// Verify each tool span exists and has correct attributes
	toolNames := make(map[string]bool)
	for _, child := range iterSpan.Children {
		toolNames[child.Name] = true
		switch child.Name {
		case "read":
			if child.Attributes["file_path"] != "file1.go" {
				t.Errorf("read tool file_path = %q, want %q", child.Attributes["file_path"], "file1.go")
			}
		case "write":
			if child.Attributes["file_path"] != "file2.go" {
				t.Errorf("write tool file_path = %q, want %q", child.Attributes["file_path"], "file2.go")
			}
		case "shell":
			if child.Attributes["command"] != "go test" {
				t.Errorf("shell tool command = %q, want %q", child.Attributes["command"], "go test")
			}
		}
	}

	if !toolNames["read"] {
		t.Error("tool span 'read' should exist")
	}
	if !toolNames["write"] {
		t.Error("tool span 'write' should exist")
	}
	if !toolNames["shell"] {
		t.Error("tool span 'shell' should exist")
	}
}
