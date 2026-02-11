package trace

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// EventType identifies the kind of trace event
type EventType string

const (
	EventLoopStart      EventType = "loop_start"       // Ralph loop begins
	EventLoopEnd        EventType = "loop_end"          // Ralph loop completes
	EventIterationStart EventType = "iteration_start"   // Starting work on a bead
	EventIterationEnd   EventType = "iteration_end"     // Bead outcome assessed
	EventToolStart      EventType = "tool_start"        // Agent tool call started
	EventToolEnd        EventType = "tool_end"          // Agent tool call completed
)

// TraceEvent represents a single event in a ralph loop trace
type TraceEvent struct {
	TraceID    string            `json:"trace_id"`    // Unique ID for the entire ralph loop
	SpanID     string            `json:"span_id"`     // Unique ID for this span
	ParentID   string            `json:"parent_id"`   // Parent span ID (empty for root)
	Type       EventType         `json:"type"`        // Event type
	Name       string            `json:"name"`        // Human-readable name (bead ID, tool name, etc.)
	Timestamp  time.Time         `json:"timestamp"`   // When the event occurred
	Attributes map[string]string `json:"attributes"`  // Additional metadata
}

// NewTraceID generates a random 16-byte trace ID as hex string (32 characters)
func NewTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// NewSpanID generates a random 8-byte span ID as hex string (16 characters)
func NewSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
