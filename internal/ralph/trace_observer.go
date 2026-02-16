package ralph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/trace"
)

// TracingObserver implements ProgressObserver and exports traces via OTLP.
// Use this for headless CLI execution with tracing support.
type TracingObserver struct {
	NoopObserver
	manager *trace.Manager

	mu           sync.Mutex
	traceID      string
	loopSpanID   string            // SpanID of the loop span
	iterSpanID   string            // Current iteration span ID
	iterNum      int               // Current iteration number
	toolSpans    map[string]string // tool call ID → span ID
}

// NewTracingObserver creates a TracingObserver that exports to OTLP.
// Set OTEL_EXPORTER_OTLP_ENDPOINT to enable export (e.g., "http://localhost:4318").
// Returns a no-op observer if OTLP is not configured.
func NewTracingObserver() *TracingObserver {
	return &TracingObserver{
		manager:   trace.NewManager(10),
		toolSpans: make(map[string]string),
	}
}

// OnLoopStart begins a new trace.
func (o *TracingObserver) OnLoopStart(rootBead string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	traceID := trace.NewTraceID()
	loopSpanID := trace.NewSpanID()
	o.traceID = traceID
	o.loopSpanID = loopSpanID
	o.iterNum = 0

	event := trace.TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      trace.EventLoopStart,
		Name:      "ralph-loop",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"root_bead": rootBead,
		},
	}

	o.manager.HandleEvent(event)
}

// OnBeadStart begins an iteration span.
func (o *TracingObserver) OnBeadStart(bead beads.Bead) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.traceID == "" {
		return
	}

	o.iterNum++
	spanID := trace.NewSpanID()
	o.iterSpanID = spanID

	event := trace.TraceEvent{
		TraceID:   o.traceID,
		SpanID:    spanID,
		ParentID:  o.loopSpanID,
		Type:      trace.EventIterationStart,
		Name:      fmt.Sprintf("iteration-%d", o.iterNum),
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"bead_id":    bead.ID,
			"bead_title": bead.Title,
			"iteration":  fmt.Sprintf("%d", o.iterNum),
		},
	}

	o.manager.HandleEvent(event)
}

// OnBeadComplete ends the current iteration span.
func (o *TracingObserver) OnBeadComplete(result BeadResult) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.traceID == "" || o.iterSpanID == "" {
		return
	}

	attrs := map[string]string{
		"outcome":     result.Outcome.String(),
		"duration_ms": fmt.Sprintf("%d", result.Duration.Milliseconds()),
	}
	if result.ChatID != "" {
		attrs["chat_id"] = result.ChatID
	}
	if result.ExitCode != 0 {
		attrs["exit_code"] = fmt.Sprintf("%d", result.ExitCode)
	}
	if result.ErrorMessage != "" {
		attrs["error"] = result.ErrorMessage
	}

	event := trace.TraceEvent{
		TraceID:    o.traceID,
		SpanID:     o.iterSpanID,
		ParentID:   o.loopSpanID,
		Type:       trace.EventIterationEnd,
		Name:       "iteration-end",
		Timestamp:  time.Now(),
		Attributes: attrs,
	}

	o.manager.HandleEvent(event)
	o.iterSpanID = ""
}

// OnLoopEnd completes the trace and triggers OTLP export.
func (o *TracingObserver) OnLoopEnd(result *CoreResult) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.traceID == "" || o.loopSpanID == "" {
		return
	}

	attrs := map[string]string{
		"succeeded": fmt.Sprintf("%d", result.Succeeded),
		"failed":    fmt.Sprintf("%d", result.Failed),
		"questions": fmt.Sprintf("%d", result.Questions),
		"timed_out": fmt.Sprintf("%d", result.TimedOut),
		"duration":  FormatDuration(result.Duration),
	}

	event := trace.TraceEvent{
		TraceID:    o.traceID,
		SpanID:     o.loopSpanID,
		Type:       trace.EventLoopEnd,
		Name:       "ralph-loop",
		Timestamp:  time.Now(),
		Attributes: attrs,
	}

	// HandleEvent triggers OTLP export for loop_end events
	o.manager.HandleEvent(event)

	o.traceID = ""
	o.loopSpanID = ""
}

// OnToolStart begins a tool call span.
func (o *TracingObserver) OnToolStart(event ToolEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.traceID == "" {
		return
	}

	spanID := trace.NewSpanID()
	o.toolSpans[event.ID] = spanID

	// Parent is the current iteration span, or loop span if no iteration
	parentID := o.iterSpanID
	if parentID == "" {
		parentID = o.loopSpanID
	}

	traceEvent := trace.TraceEvent{
		TraceID:    o.traceID,
		SpanID:     spanID,
		ParentID:   parentID,
		Type:       trace.EventToolStart,
		Name:       event.Name,
		Timestamp:  event.Timestamp,
		Attributes: event.Attributes,
	}

	o.manager.HandleEvent(traceEvent)
}

// OnToolEnd completes a tool call span.
func (o *TracingObserver) OnToolEnd(event ToolEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.traceID == "" {
		return
	}

	spanID, ok := o.toolSpans[event.ID]
	if !ok {
		return
	}
	delete(o.toolSpans, event.ID)

	traceEvent := trace.TraceEvent{
		TraceID:    o.traceID,
		SpanID:     spanID,
		Type:       trace.EventToolEnd,
		Name:       event.Name,
		Timestamp:  event.Timestamp,
		Attributes: event.Attributes,
	}

	o.manager.HandleEvent(traceEvent)
}

// Shutdown flushes pending OTLP exports. Must be called before exit.
func (o *TracingObserver) Shutdown(ctx context.Context) error {
	return o.manager.Shutdown(ctx)
}
