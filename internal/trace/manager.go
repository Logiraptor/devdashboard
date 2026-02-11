package trace

import (
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
}

// NewManager creates a new trace manager
func NewManager(maxTraces int) *Manager {
	if maxTraces <= 0 {
		maxTraces = 10
	}
	return &Manager{
		traces:        make(map[string]*Trace),
		pendingSpans:  make(map[string]*TraceEvent),
		orphanedSpans: make(map[string][]*Span),
		recentIDs:     make([]string, 0, maxTraces),
		maxTraces:     maxTraces,
	}
}

// HandleEvent processes an incoming trace event
// - For *_start events: stores in pendingSpans
// - For *_end events: finds matching start, computes duration, creates Span
// Returns the affected Trace (for UI updates)
func (m *Manager) HandleEvent(event TraceEvent) *Trace {
	m.mu.Lock()
	defer m.mu.Unlock()

	traceID := event.TraceID
	trace, exists := m.traces[traceID]

	// Handle start events
	if event.Type == EventLoopStart || event.Type == EventIterationStart || event.Type == EventToolStart {
		// Store start event in pendingSpans
		m.pendingSpans[event.SpanID] = &event

		// If loop_start, create new trace
		if event.Type == EventLoopStart {
			if exists {
				// Trace already exists, update it
				trace.StartTime = event.Timestamp
				trace.Status = "running"
			} else {
				// Create new trace
				trace = &Trace{
					ID:        traceID,
					StartTime: event.Timestamp,
					Status:    "running",
				}
				m.traces[traceID] = trace
				m.addToRecentIDs(traceID)
			}
		}
		return trace
	}

	// Handle end events
	if event.Type == EventLoopEnd || event.Type == EventIterationEnd || event.Type == EventToolEnd {
		// Find matching start event
		startEvent, found := m.pendingSpans[event.SpanID]
		if !found {
			// No matching start event found, ignore
			return nil
		}

		// Compute duration
		duration := event.Timestamp.Sub(startEvent.Timestamp)

		// Create span
		span := &Span{
			TraceID:    event.TraceID,
			SpanID:     event.SpanID,
			ParentID:   event.ParentID,
			Name:       event.Name,
			StartTime:  startEvent.Timestamp,
			Duration:   duration,
			Attributes: make(map[string]string),
			Children:   make([]*Span, 0),
		}

		// Copy attributes from both start and end events
		if startEvent.Attributes != nil {
			for k, v := range startEvent.Attributes {
				span.Attributes[k] = v
			}
		}
		if event.Attributes != nil {
			for k, v := range event.Attributes {
				span.Attributes[k] = v
			}
		}

		// Remove from pending
		delete(m.pendingSpans, event.SpanID)

		// Handle loop_end
		if event.Type == EventLoopEnd {
			if trace != nil {
				trace.EndTime = event.Timestamp
				trace.Status = "completed"
				// Root span is the loop span
				trace.RootSpan = span
				// Attach any orphaned children waiting for this loop span
				m.attachOrphanedChildren(span)
			}
			m.callOnChange()
			return trace
		}

		// Attach to parent span's Children slice
		if trace == nil {
			// Trace doesn't exist yet, create it
			trace = &Trace{
				ID:        traceID,
				StartTime: startEvent.Timestamp,
				Status:    "running",
			}
			m.traces[traceID] = trace
			m.addToRecentIDs(traceID)
		}

		// Find parent span and attach this span as a child
		if event.ParentID == "" {
			// This is a root-level span (should only happen for loop_start/loop_end)
			if trace.RootSpan == nil {
				trace.RootSpan = span
				// Attach any orphaned children waiting for this root span
				m.attachOrphanedChildren(span)
			}
		} else {
			// Find parent in the trace tree
			parent := m.findSpanByID(trace.RootSpan, event.ParentID)
			if parent != nil {
				parent.Children = append(parent.Children, span)
				// Attach any orphaned children waiting for this parent
				m.attachOrphanedChildren(span)
			} else {
				// Parent not found yet (might be pending), store as orphaned
				m.orphanedSpans[event.ParentID] = append(m.orphanedSpans[event.ParentID], span)
			}
		}

		m.callOnChange()
		return trace
	}

	return nil
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
