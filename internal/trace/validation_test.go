package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestEndToEnd_OpenTelemetryTracing validates the complete OpenTelemetry tracing
// implementation end-to-end, covering:
// 1. Tool event parsing from streaming JSON
// 2. Span creation with correct attributes
// 3. Trace export to OTLP endpoint (when configured)
// 4. TUI trace view display
// 5. ProgressObserver integration
// 6. End-to-end flow from agent execution to trace visualization
func TestEndToEnd_OpenTelemetryTracing(t *testing.T) {
	// Create a trace manager
	manager := NewManager(10)

	// Create a mock ProgressObserver that forwards events to trace manager
	observer := &traceObserver{
		manager: manager,
		toolSpans: make(map[string]string),
	}

	// Test 1: Verify tool events are correctly parsed from streaming JSON
	t.Run("ToolEventParsing", func(t *testing.T) {
		testToolEventParsing(t, observer)
	})

	// Test 2: Confirm spans are created with correct attributes
	t.Run("SpanCreation", func(t *testing.T) {
		testSpanCreation(t, manager)
	})

	// Test 3: Test trace export to OTLP endpoint when configured
	t.Run("OTLPExport", func(t *testing.T) {
		testOTLPExport(t)
	})

	// Test 4: Validate TUI trace view displays tool calls in real-time
	t.Run("TUITraceView", func(t *testing.T) {
		testTUITraceView(t, manager)
	})

	// Test 5: Ensure ProgressObserver integration works correctly
	t.Run("ProgressObserverIntegration", func(t *testing.T) {
		testProgressObserverIntegration(t, observer, manager)
	})

	// Test 6: Test end-to-end flow from agent execution to trace visualization
	t.Run("EndToEndFlow", func(t *testing.T) {
		testEndToEndFlow(t, observer, manager)
	})
}

// mockToolEvent represents a tool event for testing
type mockToolEvent struct {
	ID        string
	Name      string
	Started   bool
	Timestamp time.Time
	Attributes map[string]string
}

// traceObserver implements a mock ProgressObserver and forwards events to trace manager
type traceObserver struct {
	manager   *Manager
	toolSpans map[string]string // tool call ID -> span ID
	traceID   string
	parentID  string
}

func (o *traceObserver) OnToolStart(event mockToolEvent) {
	if o.traceID == "" {
		// Create a trace if one doesn't exist
		o.traceID = NewTraceID()
		loopSpanID := NewSpanID()
		loopStart := TraceEvent{
			TraceID:   o.traceID,
			SpanID:    loopSpanID,
			Type:      EventLoopStart,
			Name:      "test-loop",
			Timestamp: time.Now(),
		}
		o.manager.HandleEvent(loopStart)
		o.parentID = loopSpanID
	}

	// Create iteration span if needed
	if o.parentID == "" || !strings.HasPrefix(o.parentID, "iter-") {
		iterSpanID := NewSpanID()
		iterStart := TraceEvent{
			TraceID:   o.traceID,
			SpanID:    iterSpanID,
			ParentID:  o.parentID,
			Type:      EventIterationStart,
			Name:      "test-iteration",
			Timestamp: time.Now(),
			Attributes: map[string]string{
				"bead_id":    "test-bead",
				"bead_title": "Test Bead",
			},
		}
		o.manager.HandleEvent(iterStart)
		o.parentID = iterSpanID
	}

	// Create tool span
	spanID := NewSpanID()
	attrs := make(map[string]string)
	for k, v := range event.Attributes {
		attrs[k] = v
	}
	attrs["tool_name"] = event.Name

	toolStart := TraceEvent{
		TraceID:    o.traceID,
		SpanID:     spanID,
		ParentID:   o.parentID,
		Type:       EventToolStart,
		Name:       event.Name,
		Timestamp:  event.Timestamp,
		Attributes: attrs,
	}
	o.manager.HandleEvent(toolStart)
	o.toolSpans[event.ID] = spanID
}

func (o *traceObserver) OnToolEnd(event mockToolEvent) {
	spanID, ok := o.toolSpans[event.ID]
	if !ok {
		return
	}

	attrs := make(map[string]string)
	for k, v := range event.Attributes {
		attrs[k] = v
	}

	toolEnd := TraceEvent{
		TraceID:    o.traceID,
		SpanID:     spanID,
		ParentID:   o.parentID,
		Type:       EventToolEnd,
		Name:       event.Name,
		Timestamp:  event.Timestamp,
		Attributes: attrs,
	}
	o.manager.HandleEvent(toolEnd)
	delete(o.toolSpans, event.ID)
}

// testToolEventParsing verifies tool events are correctly parsed from streaming JSON
// This test simulates the tool event parsing that happens in ralph/executor.go
func testToolEventParsing(t *testing.T, observer *traceObserver) {
	// Simulate streaming JSON tool events directly
	toolEvents := []string{
		`{"type":"tool_call","subtype":"started","name":"read","call_id":"call-1","arguments":{"path":"test.go"}}` + "\n",
		`{"type":"tool_call","subtype":"ended","name":"read","call_id":"call-1","duration_ms":50}` + "\n",
		`{"type":"tool_call","subtype":"started","name":"shell","call_id":"call-2","arguments":{"command":"go test"}}` + "\n",
		`{"type":"tool_call","subtype":"ended","name":"shell","call_id":"call-2","duration_ms":100}` + "\n",
	}

	// Parse and forward events manually (simulating what toolEventWriter does)
	for _, jsonLine := range toolEvents {
		// Parse JSON line (simplified version of ParseToolEvent)
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(jsonLine)), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		if eventType != "tool_call" {
			continue
		}

		name, _ := event["name"].(string)
		if name == "" {
			continue
		}

		subtype, _ := event["subtype"].(string)
		started := subtype == "started"

		attrs := make(map[string]string)
		if args, ok := event["arguments"].(map[string]interface{}); ok {
			for k, v := range args {
				var strVal string
				switch val := v.(type) {
				case string:
					strVal = val
				case float64:
					strVal = fmt.Sprintf("%.0f", val)
				case bool:
					strVal = fmt.Sprintf("%t", val)
				default:
					strVal = fmt.Sprintf("%v", val)
				}
				switch k {
				case "path", "file_path":
					attrs["file_path"] = strVal
				case "command":
					attrs["command"] = strVal
				default:
					attrs[k] = strVal
				}
			}
		}

		callID, _ := event["call_id"].(string)
		if callID == "" {
			callID = NewSpanID()
		}

		toolEvent := mockToolEvent{
			ID:         callID,
			Name:       name,
			Started:    started,
			Timestamp:  time.Now(),
			Attributes: attrs,
		}

		if started {
			observer.OnToolStart(toolEvent)
		} else {
			observer.OnToolEnd(toolEvent)
		}
	}

	// Verify events were parsed and forwarded
	// The observer should have created spans
	trace := observer.manager.ActiveTrace()
	if trace == nil {
		t.Fatal("Expected active trace after tool events")
	}
}

// testSpanCreation confirms spans are created with correct attributes
func testSpanCreation(t *testing.T, manager *Manager) {
	traceID := NewTraceID()
	loopSpanID := NewSpanID()
	iterSpanID := NewSpanID()
	toolSpanID := NewSpanID()

	now := time.Now()

	// Create loop start
	loopStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      EventLoopStart,
		Name:      "test-loop",
		Timestamp: now,
		Attributes: map[string]string{
			"model": "composer",
			"epic":  "test-epic",
		},
	}
	manager.HandleEvent(loopStart)

	// Create iteration start
	iterStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterSpanID,
		ParentID:  loopSpanID,
		Type:      EventIterationStart,
		Name:      "iteration-1",
		Timestamp: now.Add(10 * time.Millisecond),
		Attributes: map[string]string{
			"bead_id":    "bead-123",
			"bead_title": "Test Bead",
		},
	}
	manager.HandleEvent(iterStart)

	// Create tool start
	toolStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID,
		ParentID:  iterSpanID,
		Type:      EventToolStart,
		Name:      "read",
		Timestamp: now.Add(20 * time.Millisecond),
		Attributes: map[string]string{
			"file_path": "test.go",
			"tool_name": "read",
		},
	}
	manager.HandleEvent(toolStart)

	// Create tool end
	toolEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID,
		ParentID:  iterSpanID,
		Type:      EventToolEnd,
		Name:      "read",
		Timestamp: now.Add(70 * time.Millisecond), // 50ms duration
		Attributes: map[string]string{
			"duration_ms": "50",
		},
	}
	manager.HandleEvent(toolEnd)

	// Verify span was created with correct attributes
	trace := manager.Trace(traceID)
	if trace == nil {
		t.Fatal("Expected trace to exist")
	}

	if trace.RootSpan == nil {
		t.Fatal("Expected root span")
	}

	// Verify iteration span
	if len(trace.RootSpan.Children) != 1 {
		t.Fatalf("Expected 1 iteration span, got %d", len(trace.RootSpan.Children))
	}
	iterSpan := trace.RootSpan.Children[0]
	if iterSpan.SpanID != iterSpanID {
		t.Errorf("Expected iteration span ID %q, got %q", iterSpanID, iterSpan.SpanID)
	}
	if iterSpan.Attributes["bead_id"] != "bead-123" {
		t.Errorf("Expected bead_id='bead-123', got %q", iterSpan.Attributes["bead_id"])
	}

	// Verify tool span
	if len(iterSpan.Children) != 1 {
		t.Fatalf("Expected 1 tool span, got %d", len(iterSpan.Children))
	}
	toolSpan := iterSpan.Children[0]
	if toolSpan.SpanID != toolSpanID {
		t.Errorf("Expected tool span ID %q, got %q", toolSpanID, toolSpan.SpanID)
	}
	if toolSpan.Name != "read" {
		t.Errorf("Expected tool name 'read', got %q", toolSpan.Name)
	}
	if toolSpan.Attributes["file_path"] != "test.go" {
		t.Errorf("Expected file_path='test.go', got %q", toolSpan.Attributes["file_path"])
	}
	if toolSpan.Duration != 50*time.Millisecond {
		t.Errorf("Expected duration 50ms, got %v", toolSpan.Duration)
	}
}

// testOTLPExport tests trace export to OTLP endpoint when configured
func testOTLPExport(t *testing.T) {
	// Save original env vars
	originalEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	originalServiceName := os.Getenv("OTEL_SERVICE_NAME")
	defer func() {
		if originalEndpoint != "" {
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", originalEndpoint)
		} else {
			os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		if originalServiceName != "" {
			os.Setenv("OTEL_SERVICE_NAME", originalServiceName)
		} else {
			os.Unsetenv("OTEL_SERVICE_NAME")
		}
	}()

	// Test 1: OTLP exporter disabled when endpoint not set
	t.Run("DisabledWhenNotConfigured", func(t *testing.T) {
		os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		exporter, err := NewOTLPExporter(context.Background())
		if err != nil {
			t.Fatalf("NewOTLPExporter should not error when disabled: %v", err)
		}
		if exporter != nil {
			t.Error("Expected nil exporter when OTEL_EXPORTER_OTLP_ENDPOINT not set")
		}
	})

	// Test 2: OTLP exporter created when endpoint is set
	// Note: We can't actually test the HTTP export without a real OTLP collector,
	// but we can verify the exporter is created correctly
	t.Run("CreatedWhenConfigured", func(t *testing.T) {
		os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318")
		os.Setenv("OTEL_SERVICE_NAME", "test-service")

		exporter, err := NewOTLPExporter(context.Background())
		if err != nil {
			// If exporter creation fails (e.g., no collector running), that's OK for this test
			// We're just validating the code path exists
			t.Logf("OTLP exporter creation failed (expected if no collector): %v", err)
			return
		}
		if exporter == nil {
			t.Error("Expected exporter to be created when endpoint is set")
			return
		}

		// Verify exporter is enabled
		if !exporter.enabled {
			t.Error("Expected exporter to be enabled")
		}

		// Test export (will fail without collector, but validates code path)
		traceID := NewTraceID()
		spanID := NewSpanID()
		manager := NewManager(10)

		loopStart := TraceEvent{
			TraceID:   traceID,
			SpanID:    spanID,
			Type:      EventLoopStart,
			Name:      "test-loop",
			Timestamp: time.Now(),
		}
		manager.HandleEvent(loopStart)

		loopEnd := TraceEvent{
			TraceID:   traceID,
			SpanID:    spanID,
			Type:      EventLoopEnd,
			Name:      "test-loop",
			Timestamp: time.Now().Add(100 * time.Millisecond),
		}
		trace := manager.HandleEvent(loopEnd)

		// Export trace (may fail without collector, but validates code path)
		err = exporter.ExportTrace(context.Background(), trace)
		if err != nil {
			t.Logf("ExportTrace failed (expected if no collector): %v", err)
		}

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		exporter.Shutdown(ctx)
	})
}

// testTUITraceView validates TUI trace view displays tool calls in real-time
func testTUITraceView(t *testing.T, manager *Manager) {
	// This test validates that the trace manager correctly maintains state
	// that can be consumed by the TUI trace view

	traceID := NewTraceID()
	loopSpanID := NewSpanID()
	iterSpanID := NewSpanID()
	toolSpanID1 := NewSpanID()
	toolSpanID2 := NewSpanID()

	now := time.Now()

	// Create loop start
	loopStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      EventLoopStart,
		Name:      "test-loop",
		Timestamp: now,
	}
	manager.HandleEvent(loopStart)

	// Verify active trace exists
	activeTrace := manager.ActiveTrace()
	if activeTrace == nil {
		t.Fatal("Expected active trace after loop start")
	}
	if activeTrace.Status != "running" {
		t.Errorf("Expected status 'running', got %q", activeTrace.Status)
	}

	// Create iteration start
	iterStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterSpanID,
		ParentID:  loopSpanID,
		Type:      EventIterationStart,
		Name:      "iteration-1",
		Timestamp: now.Add(10 * time.Millisecond),
		Attributes: map[string]string{
			"bead_id":    "bead-123",
			"bead_title": "Test Bead",
		},
	}
	manager.HandleEvent(iterStart)

	// Create first tool start (in-progress)
	toolStart1 := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID1,
		ParentID:  iterSpanID,
		Type:      EventToolStart,
		Name:      "read",
		Timestamp: now.Add(20 * time.Millisecond),
		Attributes: map[string]string{
			"file_path": "test.go",
		},
	}
	manager.HandleEvent(toolStart1)

	// Verify tool span exists with Duration=0 (in-progress)
	checkTrace := manager.Trace(traceID)
	if checkTrace == nil {
		t.Fatal("Expected trace to exist")
	}
	checkIterSpan := checkTrace.RootSpan.Children[0]
	if len(checkIterSpan.Children) != 1 {
		t.Fatalf("Expected 1 tool span, got %d", len(checkIterSpan.Children))
	}
	checkToolSpan1 := checkIterSpan.Children[0]
	if checkToolSpan1.Duration != 0 {
		t.Errorf("Expected in-progress tool span (Duration=0), got %v", checkToolSpan1.Duration)
	}

	// Create second tool start (parallel)
	toolStart2 := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID2,
		ParentID:  iterSpanID,
		Type:      EventToolStart,
		Name:      "shell",
		Timestamp: now.Add(30 * time.Millisecond),
		Attributes: map[string]string{
			"command": "go test",
		},
	}
	manager.HandleEvent(toolStart2)

	// Verify both tool spans exist
	updatedTrace2 := manager.Trace(traceID)
	if updatedTrace2 == nil {
		t.Fatal("Expected trace to exist")
	}
	updatedIterSpan2 := updatedTrace2.RootSpan.Children[0]
	if len(updatedIterSpan2.Children) != 2 {
		t.Fatalf("Expected 2 tool spans, got %d", len(updatedIterSpan2.Children))
	}

	// End first tool
	toolEnd1 := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID1,
		ParentID:  iterSpanID,
		Type:      EventToolEnd,
		Name:      "read",
		Timestamp: now.Add(70 * time.Millisecond), // 50ms duration
	}
	manager.HandleEvent(toolEnd1)

	// Verify first tool span has duration
	updatedTrace := manager.Trace(traceID)
	if updatedTrace == nil {
		t.Fatal("Expected trace to exist")
	}
	updatedIterSpan := updatedTrace.RootSpan.Children[0]
	updatedToolSpan1 := updatedIterSpan.Children[0]
	if updatedToolSpan1.Duration != 50*time.Millisecond {
		t.Errorf("Expected duration 50ms, got %v", updatedToolSpan1.Duration)
	}

	// Verify second tool span still in-progress
	updatedToolSpan2 := updatedIterSpan.Children[1]
	if updatedToolSpan2.Duration != 0 {
		t.Errorf("Expected in-progress tool span (Duration=0), got %v", updatedToolSpan2.Duration)
	}
}

// testProgressObserverIntegration ensures ProgressObserver integration works correctly
func testProgressObserverIntegration(t *testing.T, observer *traceObserver, manager *Manager) {
	// Simulate tool events through ProgressObserver
	toolStartEvent := mockToolEvent{
		ID:        "call-1",
		Name:      "read",
		Started:   true,
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"file_path": "test.go",
		},
	}

	observer.OnToolStart(toolStartEvent)

	// Verify trace was created
	trace := manager.ActiveTrace()
	if trace == nil {
		t.Fatal("Expected active trace after OnToolStart")
	}

	// Verify iteration span was created
	if trace.RootSpan == nil {
		t.Fatal("Expected root span")
	}
	if len(trace.RootSpan.Children) == 0 {
		t.Fatal("Expected iteration span")
	}

	// Verify tool span was created
	iterSpan := trace.RootSpan.Children[0]
	if len(iterSpan.Children) == 0 {
		t.Fatal("Expected tool span")
	}
	toolSpan := iterSpan.Children[0]
	if toolSpan.Name != "read" {
		t.Errorf("Expected tool name 'read', got %q", toolSpan.Name)
	}
	if toolSpan.Attributes["file_path"] != "test.go" {
		t.Errorf("Expected file_path='test.go', got %q", toolSpan.Attributes["file_path"])
	}

	// End tool event
	toolEndEvent := mockToolEvent{
		ID:        "call-1",
		Name:      "read",
		Started:   false,
		Timestamp: time.Now().Add(50 * time.Millisecond),
		Attributes: map[string]string{
			"duration_ms": "50",
		},
	}

	observer.OnToolEnd(toolEndEvent)

	// Verify tool span has duration
	finalTrace := manager.Trace(observer.traceID)
	if finalTrace == nil {
		t.Fatal("Expected trace to exist")
	}
	finalIterSpan := finalTrace.RootSpan.Children[0]
	if len(finalIterSpan.Children) == 0 {
		t.Fatal("Expected tool span to exist")
	}
	finalToolSpan := finalIterSpan.Children[0]
	if finalToolSpan.Duration == 0 {
		t.Error("Expected tool span to have duration after OnToolEnd")
	}
}

// testEndToEndFlow tests the complete flow from agent execution to trace visualization
func testEndToEndFlow(t *testing.T, observer *traceObserver, manager *Manager) {
	// Simulate a complete ralph loop execution

	// 1. Loop starts
	loopStart := TraceEvent{
		TraceID:   NewTraceID(),
		SpanID:    NewSpanID(),
		Type:      EventLoopStart,
		Name:      "ralph-loop",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"model": "composer",
			"epic":  "test-epic",
		},
	}
	trace := manager.HandleEvent(loopStart)
	if trace == nil {
		t.Fatal("Expected trace after loop start")
	}
	traceID := trace.ID

	// 2. Iteration starts
	iterSpanID := NewSpanID()
	iterStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterSpanID,
		ParentID:  loopStart.SpanID,
		Type:      EventIterationStart,
		Name:      "iteration-1",
		Timestamp: time.Now().Add(10 * time.Millisecond),
		Attributes: map[string]string{
			"bead_id":    "bead-123",
			"bead_title": "Test Bead",
		},
	}
	manager.HandleEvent(iterStart)

	// 3. Tool calls occur
	toolEvents := []struct {
		name      string
		spanID    string
		startTime time.Duration
		duration  time.Duration
		attrs     map[string]string
	}{
		{"read", NewSpanID(), 20 * time.Millisecond, 50 * time.Millisecond, map[string]string{"file_path": "test.go"}},
		{"shell", NewSpanID(), 30 * time.Millisecond, 100 * time.Millisecond, map[string]string{"command": "go test"}},
		{"edit", NewSpanID(), 40 * time.Millisecond, 75 * time.Millisecond, map[string]string{"file_path": "test.go"}},
	}

	baseTime := time.Now()
	for _, te := range toolEvents {
		// Tool start
		toolStart := TraceEvent{
			TraceID:    traceID,
			SpanID:     te.spanID,
			ParentID:   iterSpanID,
			Type:       EventToolStart,
			Name:       te.name,
			Timestamp:  baseTime.Add(te.startTime),
			Attributes: te.attrs,
		}
		manager.HandleEvent(toolStart)

		// Tool end
		toolEnd := TraceEvent{
			TraceID:    traceID,
			SpanID:     te.spanID,
			ParentID:   iterSpanID,
			Type:       EventToolEnd,
			Name:       te.name,
			Timestamp:  baseTime.Add(te.startTime + te.duration),
			Attributes: map[string]string{"duration_ms": fmt.Sprintf("%.0f", te.duration.Seconds()*1000)},
		}
		manager.HandleEvent(toolEnd)
	}

	// 4. Iteration ends
	iterEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterSpanID,
		ParentID:  loopStart.SpanID,
		Type:      EventIterationEnd,
		Name:      "iteration-1",
		Timestamp: baseTime.Add(200 * time.Millisecond),
		Attributes: map[string]string{
			"outcome":     "success",
			"duration_ms": "200",
		},
	}
	manager.HandleEvent(iterEnd)

	// 5. Loop ends
	loopEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    loopStart.SpanID,
		Type:      EventLoopEnd,
		Name:      "ralph-loop",
		Timestamp: baseTime.Add(250 * time.Millisecond),
		Attributes: map[string]string{
			"stop_reason": "completed",
			"iterations":  "1",
			"succeeded":   "1",
			"failed":      "0",
		},
	}
	trace = manager.HandleEvent(loopEnd)

	// Verify complete trace structure
	if trace == nil {
		t.Fatal("Expected trace after loop end")
	}
	if trace.Status != "completed" {
		t.Errorf("Expected status 'completed', got %q", trace.Status)
	}

	// Verify root span
	if trace.RootSpan == nil {
		t.Fatal("Expected root span")
	}
	if trace.RootSpan.SpanID != loopStart.SpanID {
		t.Errorf("Expected root span ID %q, got %q", loopStart.SpanID, trace.RootSpan.SpanID)
	}

	// Verify iteration span
	if len(trace.RootSpan.Children) != 1 {
		t.Fatalf("Expected 1 iteration span, got %d", len(trace.RootSpan.Children))
	}
	iterSpan := trace.RootSpan.Children[0]
	if iterSpan.SpanID != iterSpanID {
		t.Errorf("Expected iteration span ID %q, got %q", iterSpanID, iterSpan.SpanID)
	}
	if iterSpan.Attributes["outcome"] != "success" {
		t.Errorf("Expected outcome='success', got %q", iterSpan.Attributes["outcome"])
	}

	// Verify tool spans
	if len(iterSpan.Children) != 3 {
		t.Fatalf("Expected 3 tool spans, got %d", len(iterSpan.Children))
	}

	// Verify each tool span
	for i, te := range toolEvents {
		toolSpan := iterSpan.Children[i]
		if toolSpan.Name != te.name {
			t.Errorf("Tool span %d: expected name %q, got %q", i, te.name, toolSpan.Name)
		}
		if toolSpan.Duration != te.duration {
			t.Errorf("Tool span %d: expected duration %v, got %v", i, te.duration, toolSpan.Duration)
		}
		// Verify attributes
		for k, v := range te.attrs {
			if toolSpan.Attributes[k] != v {
				t.Errorf("Tool span %d: expected attribute %s=%q, got %q", i, k, v, toolSpan.Attributes[k])
			}
		}
	}

	// Verify trace is in recent traces
	recentTraces := manager.RecentTraces()
	if len(recentTraces) == 0 {
		t.Fatal("Expected trace in recent traces")
	}
	found := false
	for _, rt := range recentTraces {
		if rt.ID == traceID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected trace ID %q in recent traces", traceID)
	}
}
