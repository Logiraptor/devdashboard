package trace

import (
	"context"
	"sync"
	"time"
)

// Span represents a completed span with start time and duration
type Span struct {
	TraceID    string
	SpanID     string
	ParentID   string
	Name       string
	StartTime  time.Time
	Duration   time.Duration
	Attributes map[string]string
	Children   []*Span // Nested spans
}

// Trace represents a complete ralph loop trace
type Trace struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	RootSpan  *Span
	Status    string // "running" or "completed"
}

// Manager stores and manages traces
type Manager struct {
	mu           sync.RWMutex
	traces       map[string]*Trace      // traceID -> Trace
	pendingSpans map[string]*TraceEvent // spanID -> start event (waiting for end)
	orphanedSpans map[string][]*Span    // parentID -> []Span (spans waiting for parent)
	recentIDs    []string               // Ring buffer of recent trace IDs
	maxTraces    int                    // Max traces to keep (default 10)
	onChange     func()                 // Callback when trace state changes
	exporter     *OTLPExporter           // OTLP exporter for completed traces
}

// NewManager creates a new trace manager
func NewManager(maxTraces int) *Manager {
	if maxTraces <= 0 {
		maxTraces = 10
	}
	exporter, _ := NewOTLPExporter(context.Background())
	return &Manager{
		traces:        make(map[string]*Trace),
		pendingSpans:  make(map[string]*TraceEvent),
		orphanedSpans: make(map[string][]*Span),
		recentIDs:     make([]string, 0, maxTraces),
		maxTraces:     maxTraces,
		exporter:      exporter,
	}
}

// HandleEvent processes an incoming trace event
// - For *_start events: creates span immediately with Duration=0 (in-progress)
// - For *_end events: finds matching span and updates Duration
// Returns the affected Trace (for UI updates)
func (m *Manager) HandleEvent(event TraceEvent) *Trace {
	m.mu.Lock()
	defer m.mu.Unlock()

	traceID := event.TraceID
	trace, exists := m.traces[traceID]

	// Handle start events - create span immediately for live viewing
	if event.Type == EventLoopStart || event.Type == EventIterationStart || event.Type == EventToolStart {
		return m.handleStartEvent(event, trace, traceID, exists)
	}

	// Handle end events - find existing span and update Duration
	if event.Type == EventLoopEnd || event.Type == EventIterationEnd || event.Type == EventToolEnd {
		return m.handleEndEvent(event, trace)
	}

	return nil
}

// handleStartEvent processes loop/iteration/tool start events
// Must be called with m.mu.Lock() held
func (m *Manager) handleStartEvent(event TraceEvent, trace *Trace, traceID string, exists bool) *Trace {
	// Store start event in pendingSpans (for duration calculation later)
	m.pendingSpans[event.SpanID] = &event

	// Create span immediately with Duration=0 (indicates in-progress)
	span := &Span{
		TraceID:    event.TraceID,
		SpanID:     event.SpanID,
		ParentID:   event.ParentID,
		Name:       event.Name,
		StartTime:  event.Timestamp,
		Duration:   0, // In-progress
		Attributes: make(map[string]string),
		Children:   make([]*Span, 0),
	}
	if event.Attributes != nil {
		for k, v := range event.Attributes {
			span.Attributes[k] = v
		}
	}

	// If loop_start, create/update trace and set as RootSpan
	if event.Type == EventLoopStart {
		if exists {
			// Trace already exists, update it
			trace.StartTime = event.Timestamp
			trace.Status = "running"
			trace.RootSpan = span
		} else {
			// Create new trace with RootSpan
			trace = &Trace{
				ID:        traceID,
				StartTime: event.Timestamp,
				Status:    "running",
				RootSpan:  span,
			}
			m.traces[traceID] = trace
			m.addToRecentIDs(traceID)
		}
		// Attach any orphaned children waiting for the loop span
		m.attachOrphanedChildren(span)
		m.callOnChange()
		return trace
	}

	// For iteration/tool start, attach to parent
	if trace == nil {
		// Trace doesn't exist yet, create it
		trace = &Trace{
			ID:        traceID,
			StartTime: event.Timestamp,
			Status:    "running",
		}
		m.traces[traceID] = trace
		m.addToRecentIDs(traceID)
	}

	// Find parent and attach
	if event.ParentID != "" {
		if trace.RootSpan != nil {
			parent := m.findSpanByID(trace.RootSpan, event.ParentID)
			if parent != nil {
				parent.Children = append(parent.Children, span)
			} else {
				// Parent not found yet, store as orphaned
				m.orphanedSpans[event.ParentID] = append(m.orphanedSpans[event.ParentID], span)
			}
		} else {
			// RootSpan doesn't exist yet, store as orphaned
			m.orphanedSpans[event.ParentID] = append(m.orphanedSpans[event.ParentID], span)
		}
	} else {
		// No ParentID - this is a root-level span
		if trace.RootSpan == nil {
			trace.RootSpan = span
			m.attachOrphanedChildren(span)
		}
	}

	m.callOnChange()
	return trace
}

// handleEndEvent processes loop/iteration/tool end events
// Must be called with m.mu.Lock() held
func (m *Manager) handleEndEvent(event TraceEvent, trace *Trace) *Trace {
	// Find matching start event
	startEvent, found := m.pendingSpans[event.SpanID]
	if !found {
		// No matching start event found, ignore
		return nil
	}

	// Compute duration
	duration := event.Timestamp.Sub(startEvent.Timestamp)

	// Remove from pending
	delete(m.pendingSpans, event.SpanID)

	// Find the existing span in the tree and update it
	if trace != nil && trace.RootSpan != nil {
		var span *Span
		if trace.RootSpan.SpanID == event.SpanID {
			span = trace.RootSpan
		} else {
			span = m.findSpanByID(trace.RootSpan, event.SpanID)
		}

		if span != nil {
			// Update span with duration and end event attributes
			span.Duration = duration
			if event.Attributes != nil {
				for k, v := range event.Attributes {
					span.Attributes[k] = v
				}
			}
		}
	}

	// Handle loop_end
	if event.Type == EventLoopEnd {
		if trace != nil {
			trace.EndTime = event.Timestamp
			trace.Status = "completed"
			// Export to OTLP synchronously - this is the final event and we must
			// ensure the trace is exported before the process exits
			if m.exporter != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := m.exporter.ExportTrace(ctx, trace); err != nil {
					// TODO: Surface export errors through observer pattern (OnError callback)
					// or store in TraceManager for later retrieval. log.Printf interferes
					// with bubbletea rendering.
					_ = err // Silently ignore for now
				}
				cancel()
			}
		}
		m.callOnChange()
		return trace
	}

	m.callOnChange()
	return trace
}

// findSpanByID recursively searches for a span by ID in the trace tree
func (m *Manager) findSpanByID(root *Span, spanID string) *Span {
	if root == nil {
		return nil
	}
	if root.SpanID == spanID {
		return root
	}
	for _, child := range root.Children {
		if found := m.findSpanByID(child, spanID); found != nil {
			return found
		}
	}
	return nil
}

// attachOrphanedChildren attaches any orphaned children waiting for the given parent span
// Recursively attaches orphaned children of attached children as well
func (m *Manager) attachOrphanedChildren(parent *Span) {
	if orphans, exists := m.orphanedSpans[parent.SpanID]; exists {
		parent.Children = append(parent.Children, orphans...)
		delete(m.orphanedSpans, parent.SpanID)
		// Recursively attach orphaned children of the newly attached children
		for _, child := range orphans {
			m.attachOrphanedChildren(child)
		}
	}
}

// addToRecentIDs adds a trace ID to the recent list, evicting old ones if needed
func (m *Manager) addToRecentIDs(traceID string) {
	// Check if already in list
	for i, id := range m.recentIDs {
		if id == traceID {
			// Move to end
			m.recentIDs = append(append(m.recentIDs[:i], m.recentIDs[i+1:]...), traceID)
			return
		}
	}

	// Add to end
	m.recentIDs = append(m.recentIDs, traceID)

	// Evict old traces if exceeding maxTraces
	if len(m.recentIDs) > m.maxTraces {
		// Remove oldest trace
		oldestID := m.recentIDs[0]
		m.recentIDs = m.recentIDs[1:]
		delete(m.traces, oldestID)
	}
}

// callOnChange calls the onChange callback if set (must be called with lock held)
func (m *Manager) callOnChange() {
	if m.onChange != nil {
		m.onChange()
	}
}

// GetTrace returns a trace by ID
func (m *Manager) GetTrace(id string) *Trace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.traces[id]
}

// GetActiveTrace returns the currently running trace (if any)
func (m *Manager) GetActiveTrace() *Trace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, trace := range m.traces {
		if trace.Status == "running" {
			return trace
		}
	}
	return nil
}

// GetRecentTraces returns recent traces (newest first)
func (m *Manager) GetRecentTraces() []*Trace {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Trace, 0, len(m.recentIDs))
	// Iterate in reverse to get newest first
	for i := len(m.recentIDs) - 1; i >= 0; i-- {
		if trace, exists := m.traces[m.recentIDs[i]]; exists {
			result = append(result, trace)
		}
	}
	return result
}

// SetOnChange sets callback for state changes (thread-safe)
func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onChange = fn
}

// Shutdown flushes pending exports and closes the OTLP exporter.
// Must be called before process exit to ensure traces are exported.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	exporter := m.exporter
	m.mu.Unlock()

	if exporter != nil {
		return exporter.Shutdown(ctx)
	}
	return nil
}
