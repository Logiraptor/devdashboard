package ralph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"devdeploy/internal/trace"
)

// DefaultTraceURL is the default devdeploy trace server URL
const DefaultTraceURL = "http://localhost:9876/traces"

// TraceClient sends trace events to devdeploy
type TraceClient struct {
	url      string
	client   *http.Client
	traceID  string // Current trace ID (set at loop start)
	parentID string // Current parent span ID (for nesting)
	enabled  bool   // False if server unavailable
	mu       sync.Mutex
}

// NewTraceClient creates a new trace client
// Reads URL from DEVDEPLOY_TRACE_URL env var, defaults to DefaultTraceURL
// Pings server to check availability; disables itself if unreachable
func NewTraceClient() *TraceClient {
	url := os.Getenv("DEVDEPLOY_TRACE_URL")
	if url == "" {
		url = DefaultTraceURL
	}

	client := &http.Client{
		Timeout: 2 * time.Second, // Short timeout for non-blocking sends
	}

	tc := &TraceClient{
		url:    url,
		client: client,
		enabled: true,
	}

	// Ping server to check availability
	go func() {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			tc.mu.Lock()
			tc.enabled = false
			tc.mu.Unlock()
			log.Printf("trace: failed to create ping request: %v", err)
			return
		}

		// Use a very short timeout for the ping
		pingClient := &http.Client{Timeout: 1 * time.Second}
		resp, err := pingClient.Do(req)
		if err != nil {
			tc.mu.Lock()
			tc.enabled = false
			tc.mu.Unlock()
			log.Printf("trace: server unavailable at %s, disabling trace client: %v", url, err)
			return
		}
		resp.Body.Close()

		// Server is available
		tc.mu.Lock()
		tc.enabled = true
		tc.mu.Unlock()
	}()

	return tc
}

// StartLoop begins a new trace, returns the trace ID
func (c *TraceClient) StartLoop(model, epic, workdir string, maxIterations int) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	traceID := trace.NewTraceID()
	c.traceID = traceID
	c.parentID = "" // Reset parent for new trace

	event := trace.TraceEvent{
		TraceID:   traceID,
		SpanID:    trace.NewSpanID(),
		ParentID:  "",
		Type:      trace.EventLoopStart,
		Name:      fmt.Sprintf("ralph-loop-%s", epic),
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"model":          model,
			"epic":           epic,
			"workdir":        workdir,
			"max_iterations": fmt.Sprintf("%d", maxIterations),
		},
	}

	c.send(event)
	return traceID
}

// EndLoop completes the current trace
func (c *TraceClient) EndLoop(stopReason string, iterations, succeeded, failed int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.traceID == "" {
		return // No active trace
	}

	event := trace.TraceEvent{
		TraceID:   c.traceID,
		SpanID:    trace.NewSpanID(),
		ParentID:  "",
		Type:      trace.EventLoopEnd,
		Name:      "ralph-loop-end",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"stop_reason": stopReason,
			"iterations":  fmt.Sprintf("%d", iterations),
			"succeeded":   fmt.Sprintf("%d", succeeded),
			"failed":      fmt.Sprintf("%d", failed),
		},
	}

	c.send(event)
	c.traceID = "" // Clear trace ID
}

// StartIteration begins an iteration span, returns span ID
func (c *TraceClient) StartIteration(beadID, beadTitle string, iterNum int) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.traceID == "" {
		return "" // No active trace
	}

	spanID := trace.NewSpanID()
	c.parentID = spanID // Set as parent for nested tool calls

	event := trace.TraceEvent{
		TraceID:   c.traceID,
		SpanID:    spanID,
		ParentID:  "",
		Type:      trace.EventIterationStart,
		Name:      fmt.Sprintf("iteration-%d", iterNum),
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"bead_id":    beadID,
			"bead_title": beadTitle,
			"iteration":  fmt.Sprintf("%d", iterNum),
		},
	}

	c.send(event)
	return spanID
}

// EndIteration completes an iteration span
func (c *TraceClient) EndIteration(spanID string, outcome string, durationMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.traceID == "" || spanID == "" {
		return
	}

	event := trace.TraceEvent{
		TraceID:   c.traceID,
		SpanID:    spanID,
		ParentID:  "",
		Type:      trace.EventIterationEnd,
		Name:      "iteration-end",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"outcome":       outcome,
			"duration_ms":   fmt.Sprintf("%d", durationMs),
		},
	}

	c.send(event)
	c.parentID = "" // Clear parent after iteration ends
}

// StartTool begins a tool span, returns span ID
// toolName: "read", "edit", "shell", "search", "grep"
// attrs: tool-specific attributes (file_path, command, query, etc.)
func (c *TraceClient) StartTool(toolName string, attrs map[string]string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.traceID == "" {
		return "" // No active trace
	}

	spanID := trace.NewSpanID()

	event := trace.TraceEvent{
		TraceID:   c.traceID,
		SpanID:    spanID,
		ParentID:  c.parentID, // Use current parent for nesting
		Type:      trace.EventToolStart,
		Name:      toolName,
		Timestamp: time.Now(),
		Attributes: attrs,
	}

	c.send(event)
	return spanID
}

// EndTool completes a tool span
func (c *TraceClient) EndTool(spanID string, attrs map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.traceID == "" || spanID == "" {
		return
	}

	event := trace.TraceEvent{
		TraceID:   c.traceID,
		SpanID:    spanID,
		ParentID:  c.parentID,
		Type:      trace.EventToolEnd,
		Name:      "tool-end",
		Timestamp: time.Now(),
		Attributes: attrs,
	}

	c.send(event)
}

// SetParent sets the current parent span ID (for nesting tool calls under iterations)
func (c *TraceClient) SetParent(spanID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parentID = spanID
}

// send POSTs an event to the trace server (fire-and-forget, logs errors)
func (c *TraceClient) send(event trace.TraceEvent) {
	// Check if enabled (non-blocking check)
	c.mu.Lock()
	enabled := c.enabled
	c.mu.Unlock()

	if !enabled {
		return // Silently skip if disabled
	}

	// Send asynchronously to avoid blocking
	go func() {
		jsonData, err := json.Marshal(event)
		if err != nil {
			log.Printf("trace: failed to marshal event: %v", err)
			return
		}

		req, err := http.NewRequest("POST", c.url, bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("trace: failed to create request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			// If send fails, disable client to avoid spam
			c.mu.Lock()
			if c.enabled {
				c.enabled = false
				log.Printf("trace: send failed, disabling trace client: %v", err)
			}
			c.mu.Unlock()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			log.Printf("trace: server returned status %d", resp.StatusCode)
		}
	}()
}
