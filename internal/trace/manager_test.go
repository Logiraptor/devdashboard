package trace

import (
	"sync"
	"testing"
	"time"
)

func TestNewManager_Defaults(t *testing.T) {
	m := NewManager(0)
	if m.maxTraces != 10 {
		t.Errorf("NewManager(0): expected maxTraces=10, got %d", m.maxTraces)
	}
	if m.traces == nil || m.pendingSpans == nil {
		t.Error("NewManager: expected maps to be initialized")
	}
}

func TestNewManager_CustomMaxTraces(t *testing.T) {
	m := NewManager(5)
	if m.maxTraces != 5 {
		t.Errorf("NewManager(5): expected maxTraces=5, got %d", m.maxTraces)
	}
}

func TestHandleEvent_LoopStart_CreatesTrace(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()

	event := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventLoopStart,
		Name:      "test-loop",
		Timestamp: time.Now(),
	}

	trace := m.HandleEvent(event)
	if trace == nil {
		t.Fatal("HandleEvent(loop_start): expected trace, got nil")
	}
	if trace.ID != traceID {
		t.Errorf("HandleEvent(loop_start): expected trace ID %q, got %q", traceID, trace.ID)
	}
	if trace.Status != "running" {
		t.Errorf("HandleEvent(loop_start): expected status 'running', got %q", trace.Status)
	}
	if !trace.StartTime.Equal(event.Timestamp) {
		t.Errorf("HandleEvent(loop_start): expected StartTime %v, got %v", event.Timestamp, trace.StartTime)
	}
}

func TestHandleEvent_PairsStartEnd(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()
	startTime := time.Now()
	endTime := startTime.Add(100 * time.Millisecond)

	startEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventIterationStart,
		Name:      "test-iteration",
		Timestamp: startTime,
		Attributes: map[string]string{"bead": "test-123"},
	}

	endEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		ParentID:  "",
		Type:      EventIterationEnd,
		Name:      "test-iteration",
		Timestamp: endTime,
		Attributes: map[string]string{"outcome": "completed"},
	}

	// Process start event
	m.HandleEvent(startEvent)

	// Process end event
	trace := m.HandleEvent(endEvent)
	if trace == nil {
		t.Fatal("HandleEvent(iteration_end): expected trace, got nil")
	}

	// Check that span was created and attached
	if trace.RootSpan == nil {
		t.Fatal("HandleEvent(iteration_end): expected root span, got nil")
	}
	if trace.RootSpan.SpanID != spanID {
		t.Errorf("HandleEvent(iteration_end): expected span ID %q, got %q", spanID, trace.RootSpan.SpanID)
	}
	if trace.RootSpan.Duration != 100*time.Millisecond {
		t.Errorf("HandleEvent(iteration_end): expected duration 100ms, got %v", trace.RootSpan.Duration)
	}
	if trace.RootSpan.Attributes["bead"] != "test-123" {
		t.Errorf("HandleEvent(iteration_end): expected attribute 'bead'='test-123', got %q", trace.RootSpan.Attributes["bead"])
	}
	if trace.RootSpan.Attributes["outcome"] != "completed" {
		t.Errorf("HandleEvent(iteration_end): expected attribute 'outcome'='completed', got %q", trace.RootSpan.Attributes["outcome"])
	}
}

func TestHandleEvent_DurationCalculatedAccurately(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()
	startTime := time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	endTime := startTime.Add(250 * time.Millisecond)

	startEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventToolStart,
		Name:      "test-tool",
		Timestamp: startTime,
	}

	endEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventToolEnd,
		Name:      "test-tool",
		Timestamp: endTime,
	}

	m.HandleEvent(startEvent)
	m.HandleEvent(endEvent)

	trace := m.Trace(traceID)
	if trace == nil {
		t.Fatal("GetTrace: expected trace, got nil")
	}

	// Find the span (should be root span in this case)
	if trace.RootSpan == nil {
		t.Fatal("RootSpan: expected root span, got nil")
	}
	if trace.RootSpan.SpanID != spanID {
		t.Fatalf("RootSpan.SpanID: expected %q, got %q", spanID, trace.RootSpan.SpanID)
	}

	expectedDuration := 250 * time.Millisecond
	if trace.RootSpan.Duration != expectedDuration {
		t.Errorf("Duration: expected %v, got %v", expectedDuration, trace.RootSpan.Duration)
	}
}

func TestHandleEvent_NestedSpans(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	loopSpanID := NewSpanID()
	iterationSpanID := NewSpanID()
	toolSpanID := NewSpanID()

	now := time.Now()

	// Create loop start
	loopStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      EventLoopStart,
		Name:      "loop",
		Timestamp: now,
	}

	// Create iteration start (child of loop)
	iterationStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterationSpanID,
		ParentID:  loopSpanID,
		Type:      EventIterationStart,
		Name:      "iteration",
		Timestamp: now.Add(10 * time.Millisecond),
	}

	// Create tool start (child of iteration)
	toolStart := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID,
		ParentID:  iterationSpanID,
		Type:      EventToolStart,
		Name:      "tool",
		Timestamp: now.Add(20 * time.Millisecond),
	}

	// End events in reverse order
	toolEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    toolSpanID,
		ParentID:  iterationSpanID,
		Type:      EventToolEnd,
		Name:      "tool",
		Timestamp: now.Add(30 * time.Millisecond),
	}

	iterationEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    iterationSpanID,
		ParentID:  loopSpanID,
		Type:      EventIterationEnd,
		Name:      "iteration",
		Timestamp: now.Add(40 * time.Millisecond),
	}

	loopEnd := TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      EventLoopEnd,
		Name:      "loop",
		Timestamp: now.Add(50 * time.Millisecond),
	}

	// Process events
	m.HandleEvent(loopStart)
	m.HandleEvent(iterationStart)
	m.HandleEvent(toolStart)
	m.HandleEvent(toolEnd)
	m.HandleEvent(iterationEnd)
	m.HandleEvent(loopEnd)

	trace := m.Trace(traceID)
	if trace == nil {
		t.Fatal("GetTrace: expected trace, got nil")
	}

	if trace.RootSpan == nil {
		t.Fatal("RootSpan: expected root span, got nil")
	}

	// Check root span
	if trace.RootSpan.SpanID != loopSpanID {
		t.Errorf("RootSpan.SpanID: expected %q, got %q", loopSpanID, trace.RootSpan.SpanID)
	}

	// Check iteration span is child of loop
	if len(trace.RootSpan.Children) != 1 {
		t.Fatalf("RootSpan.Children: expected 1 child, got %d", len(trace.RootSpan.Children))
	}
	iterationSpan := trace.RootSpan.Children[0]
	if iterationSpan.SpanID != iterationSpanID {
		t.Errorf("IterationSpan.SpanID: expected %q, got %q", iterationSpanID, iterationSpan.SpanID)
	}

	// Check tool span is child of iteration
	if len(iterationSpan.Children) != 1 {
		t.Fatalf("IterationSpan.Children: expected 1 child, got %d", len(iterationSpan.Children))
	}
	toolSpan := iterationSpan.Children[0]
	if toolSpan.SpanID != toolSpanID {
		t.Errorf("ToolSpan.SpanID: expected %q, got %q", toolSpanID, toolSpan.SpanID)
	}
}

func TestHandleEvent_LoopEnd_MarksCompleted(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()
	startTime := time.Now()
	endTime := startTime.Add(100 * time.Millisecond)

	startEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventLoopStart,
		Name:      "test-loop",
		Timestamp: startTime,
	}

	endEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventLoopEnd,
		Name:      "test-loop",
		Timestamp: endTime,
	}

	m.HandleEvent(startEvent)
	trace := m.HandleEvent(endEvent)

	if trace == nil {
		t.Fatal("HandleEvent(loop_end): expected trace, got nil")
	}
	if trace.Status != "completed" {
		t.Errorf("HandleEvent(loop_end): expected status 'completed', got %q", trace.Status)
	}
	if !trace.EndTime.Equal(endTime) {
		t.Errorf("HandleEvent(loop_end): expected EndTime %v, got %v", endTime, trace.EndTime)
	}
}

func TestHandleEvent_EndWithoutStart_Ignored(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()

	endEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventIterationEnd,
		Name:      "test",
		Timestamp: time.Now(),
	}

	trace := m.HandleEvent(endEvent)
	if trace != nil {
		t.Errorf("HandleEvent(end without start): expected nil, got trace %v", trace)
	}
}

func TestGetTrace_ReturnsTrace(t *testing.T) {
	m := NewManager(10)
	traceID := NewTraceID()
	spanID := NewSpanID()

	event := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventLoopStart,
		Name:      "test",
		Timestamp: time.Now(),
	}

	m.HandleEvent(event)
	trace := m.Trace(traceID)

	if trace == nil {
		t.Fatal("GetTrace: expected trace, got nil")
	}
	if trace.ID != traceID {
		t.Errorf("GetTrace: expected trace ID %q, got %q", traceID, trace.ID)
	}
}

func TestGetTrace_NotFound_ReturnsNil(t *testing.T) {
	m := NewManager(10)
	trace := m.Trace("nonexistent")
	if trace != nil {
		t.Errorf("Trace(nonexistent): expected nil, got %v", trace)
	}
}

func TestGetActiveTrace_ReturnsRunningTrace(t *testing.T) {
	m := NewManager(10)
	traceID1 := NewTraceID()
	traceID2 := NewTraceID()

	// Create completed trace
	event1 := TraceEvent{
		TraceID:   traceID1,
		SpanID:    NewSpanID(),
		Type:      EventLoopStart,
		Name:      "completed",
		Timestamp: time.Now(),
	}
	m.HandleEvent(event1)
	endEvent1 := TraceEvent{
		TraceID:   traceID1,
		SpanID:    event1.SpanID,
		Type:      EventLoopEnd,
		Name:      "completed",
		Timestamp: time.Now(),
	}
	m.HandleEvent(endEvent1)

	// Create running trace
	event2 := TraceEvent{
		TraceID:   traceID2,
		SpanID:    NewSpanID(),
		Type:      EventLoopStart,
		Name:      "running",
		Timestamp: time.Now(),
	}
	m.HandleEvent(event2)

	active := m.ActiveTrace()
	if active == nil {
		t.Fatal("GetActiveTrace: expected running trace, got nil")
	}
	if active.ID != traceID2 {
		t.Errorf("GetActiveTrace: expected trace ID %q, got %q", traceID2, active.ID)
	}
}

func TestGetActiveTrace_NoRunningTrace_ReturnsNil(t *testing.T) {
	m := NewManager(10)
	active := m.ActiveTrace()
	if active != nil {
		t.Errorf("GetActiveTrace: expected nil, got %v", active)
	}
}

func TestGetRecentTraces_NewestFirst(t *testing.T) {
	m := NewManager(10)

	// Create 3 traces
	var traceIDs []string
	for i := 0; i < 3; i++ {
		traceID := NewTraceID()
		traceIDs = append(traceIDs, traceID)
		event := TraceEvent{
			TraceID:   traceID,
			SpanID:    NewSpanID(),
			Type:      EventLoopStart,
			Name:      "test",
			Timestamp: time.Now(),
		}
		m.HandleEvent(event)
	}

	recent := m.RecentTraces()
	if len(recent) != 3 {
		t.Fatalf("GetRecentTraces: expected 3 traces, got %d", len(recent))
	}

	// Should be newest first
	if recent[0].ID != traceIDs[2] {
		t.Errorf("GetRecentTraces[0]: expected newest trace %q, got %q", traceIDs[2], recent[0].ID)
	}
	if recent[2].ID != traceIDs[0] {
		t.Errorf("GetRecentTraces[2]: expected oldest trace %q, got %q", traceIDs[0], recent[2].ID)
	}
}

func TestRingBuffer_EvictsOldTraces(t *testing.T) {
	m := NewManager(3) // Max 3 traces

	// Create 5 traces
	var traceIDs []string
	for i := 0; i < 5; i++ {
		traceID := NewTraceID()
		traceIDs = append(traceIDs, traceID)
		event := TraceEvent{
			TraceID:   traceID,
			SpanID:    NewSpanID(),
			Type:      EventLoopStart,
			Name:      "test",
			Timestamp: time.Now(),
		}
		m.HandleEvent(event)
	}

	// Should only have 3 traces (newest 3)
	recent := m.RecentTraces()
	if len(recent) != 3 {
		t.Fatalf("GetRecentTraces: expected 3 traces, got %d", len(recent))
	}

	// Oldest traces should be evicted
	if m.Trace(traceIDs[0]) != nil {
		t.Error("RingBuffer: expected oldest trace to be evicted")
	}
	if m.Trace(traceIDs[1]) != nil {
		t.Error("RingBuffer: expected second oldest trace to be evicted")
	}

	// Newest traces should still exist
	if m.Trace(traceIDs[2]) == nil {
		t.Error("RingBuffer: expected third trace to exist")
	}
	if m.Trace(traceIDs[3]) == nil {
		t.Error("RingBuffer: expected fourth trace to exist")
	}
	if m.Trace(traceIDs[4]) == nil {
		t.Error("RingBuffer: expected newest trace to exist")
	}
}

func TestSetOnChange_CallbackCalled(t *testing.T) {
	m := NewManager(10)
	called := false
	var mu sync.Mutex

	m.SetOnChange(func() {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	traceID := NewTraceID()
	spanID := NewSpanID()

	// Process start event (should not call onChange)
	startEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventIterationStart,
		Name:      "test",
		Timestamp: time.Now(),
	}
	m.HandleEvent(startEvent)

	// Process end event (should call onChange)
	endEvent := TraceEvent{
		TraceID:   traceID,
		SpanID:    spanID,
		Type:      EventIterationEnd,
		Name:      "test",
		Timestamp: time.Now(),
	}
	m.HandleEvent(endEvent)

	mu.Lock()
	wasCalled := called
	mu.Unlock()

	if !wasCalled {
		t.Error("SetOnChange: expected callback to be called on end event")
	}
}

func TestConcurrentAccess_Safe(t *testing.T) {
	m := NewManager(10)
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 10

	// Create multiple traces concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			traceID := NewTraceID()

			for j := 0; j < eventsPerGoroutine; j++ {
				spanID := NewSpanID()
				startEvent := TraceEvent{
					TraceID:   traceID,
					SpanID:    spanID,
					Type:      EventIterationStart,
					Name:      "test",
					Timestamp: time.Now(),
				}
				m.HandleEvent(startEvent)

				endEvent := TraceEvent{
					TraceID:   traceID,
					SpanID:    spanID,
					Type:      EventIterationEnd,
					Name:      "test",
					Timestamp: time.Now(),
				}
				m.HandleEvent(endEvent)

				// Concurrent reads
				m.Trace(traceID)
				m.ActiveTrace()
				m.RecentTraces()
			}
		}(i)
	}

	wg.Wait()

	// Verify no panics occurred and state is consistent
	recent := m.RecentTraces()
	if len(recent) > m.maxTraces {
		t.Errorf("ConcurrentAccess: expected at most %d traces, got %d", m.maxTraces, len(recent))
	}
}
