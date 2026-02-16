package tui

import (
	"fmt"
	"sync"
	"time"

	"devdeploy/internal/trace"

	tea "github.com/charmbracelet/bubbletea"
)

// LocalTraceEmitter emits trace events as Bubble Tea messages
// instead of HTTP requests. It maintains local trace state.
type LocalTraceEmitter struct {
	manager    *trace.Manager
	program    *tea.Program // Set after TUI starts
	mu         sync.Mutex
	traceID    string
	loopSpanID string // SpanID of the loop span (for EndLoop and iteration ParentID)
	parentID   string // Current parent span ID for nesting (iteration spanID for tools)
}

// NewLocalTraceEmitter creates a new local trace emitter
func NewLocalTraceEmitter() *LocalTraceEmitter {
	return &LocalTraceEmitter{
		manager: trace.NewManager(10),
	}
}

// SetProgram sets the tea.Program for sending messages
// Must be called after tea.NewProgram() but before loop starts
func (e *LocalTraceEmitter) SetProgram(p *tea.Program) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.program = p
}

// Manager returns the trace manager for reading state
func (e *LocalTraceEmitter) Manager() *trace.Manager {
	return e.manager
}

// ActiveTrace returns the currently active trace
func (e *LocalTraceEmitter) ActiveTrace() *trace.Trace {
	return e.manager.ActiveTrace()
}

// StartLoop begins a new trace
func (e *LocalTraceEmitter) StartLoop(model, epic, workdir string, maxIterations int) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	traceID := trace.NewTraceID()
	loopSpanID := trace.NewSpanID()
	e.traceID = traceID
	e.loopSpanID = loopSpanID
	e.parentID = ""

	event := trace.TraceEvent{
		TraceID:   traceID,
		SpanID:    loopSpanID,
		Type:      trace.EventLoopStart,
		Name:      "ralph-loop",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"model":          model,
			"epic":           epic,
			"workdir":        workdir,
			"max_iterations": fmt.Sprintf("%d", maxIterations),
		},
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
	return traceID
}

// EndLoop completes the trace
func (e *LocalTraceEmitter) EndLoop(stopReason string, iterations, succeeded, failed int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.traceID == "" || e.loopSpanID == "" {
		return
	}

	event := trace.TraceEvent{
		TraceID:   e.traceID,
		SpanID:    e.loopSpanID, // Use the same SpanID from StartLoop
		Type:      trace.EventLoopEnd,
		Name:      "ralph-loop",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"stop_reason": stopReason,
			"iterations":  fmt.Sprintf("%d", iterations),
			"succeeded":   fmt.Sprintf("%d", succeeded),
			"failed":      fmt.Sprintf("%d", failed),
		},
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
	e.traceID = ""
	e.loopSpanID = ""
}

// StartIteration begins an iteration span
func (e *LocalTraceEmitter) StartIteration(beadID, beadTitle string, iterNum int) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.traceID == "" {
		return ""
	}

	spanID := trace.NewSpanID()
	e.parentID = spanID // Tool calls will be children of this

	event := trace.TraceEvent{
		TraceID:   e.traceID,
		SpanID:    spanID,
		ParentID:  e.loopSpanID, // Iterations are children of the loop span
		Type:      trace.EventIterationStart,
		Name:      fmt.Sprintf("iteration-%d", iterNum),
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"bead_id":    beadID,
			"bead_title": beadTitle,
			"iteration":  fmt.Sprintf("%d", iterNum),
		},
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
	return spanID
}

// EndIteration completes an iteration span
func (e *LocalTraceEmitter) EndIteration(spanID string, outcome string, durationMs int64) {
	e.EndIterationWithAttrs(spanID, outcome, durationMs, nil)
}

// EndIterationWithAttrs completes an iteration span with additional attributes
func (e *LocalTraceEmitter) EndIterationWithAttrs(spanID string, outcome string, durationMs int64, extraAttrs map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.traceID == "" || spanID == "" {
		return
	}

	attrs := map[string]string{
		"outcome":     outcome,
		"duration_ms": fmt.Sprintf("%d", durationMs),
	}
	// Merge extra attributes
	for k, v := range extraAttrs {
		attrs[k] = v
	}

	event := trace.TraceEvent{
		TraceID:    e.traceID,
		SpanID:     spanID,
		ParentID:   e.loopSpanID, // Iterations are children of the loop span
		Type:       trace.EventIterationEnd,
		Name:       "iteration-end",
		Timestamp:  time.Now(),
		Attributes: attrs,
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
	e.parentID = ""
}

// StartTool begins a tool call span
func (e *LocalTraceEmitter) StartTool(toolName string, attrs map[string]string) string {
	return e.StartToolWithParent(toolName, attrs, "")
}

// StartToolWithParent begins a tool call span with an explicit parent span ID.
// If parentSpanID is empty, it uses the emitter's current parentID.
func (e *LocalTraceEmitter) StartToolWithParent(toolName string, attrs map[string]string, parentSpanID string) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.traceID == "" {
		return ""
	}

	spanID := trace.NewSpanID()

	// Use explicit parent if provided, otherwise fall back to emitter's parentID
	parent := parentSpanID
	if parent == "" {
		parent = e.parentID
	}

	event := trace.TraceEvent{
		TraceID:    e.traceID,
		SpanID:     spanID,
		ParentID:   parent,
		Type:       trace.EventToolStart,
		Name:       toolName,
		Timestamp:  time.Now(),
		Attributes: attrs,
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
	return spanID
}

// EndTool completes a tool call span
func (e *LocalTraceEmitter) EndTool(spanID string, attrs map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.traceID == "" || spanID == "" {
		return
	}

	event := trace.TraceEvent{
		TraceID:    e.traceID,
		SpanID:     spanID,
		ParentID:   e.parentID,
		Type:       trace.EventToolEnd,
		Name:       "tool-end",
		Timestamp:  time.Now(),
		Attributes: attrs,
	}

	e.manager.HandleEvent(event)
	e.sendUpdate()
}

// SetParent sets the current parent span ID
func (e *LocalTraceEmitter) SetParent(spanID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.parentID = spanID
}

// TraceUpdateMsg is sent when trace state changes
type TraceUpdateMsg struct {
	Trace *trace.Trace
}

// sendUpdate sends a trace update message to the TUI
func (e *LocalTraceEmitter) sendUpdate() {
	if e.program == nil {
		return
	}
	activeTrace := e.manager.ActiveTrace()
	if activeTrace != nil {
		e.program.Send(TraceUpdateMsg{Trace: activeTrace})
	}
}
