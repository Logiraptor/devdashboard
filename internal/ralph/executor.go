// Package ralph implements the autonomous agent work loop.
package ralph

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the default per-iteration agent timeout.
const DefaultTimeout = 10 * time.Minute

// AgentResult holds the outcome of a single agent invocation.
type AgentResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool // true if the agent was killed due to timeout
}

// CommandFactory builds an *exec.Cmd for the given context, working directory,
// and arguments. The default factory uses exec.CommandContext with "agent" as
// the binary. Tests can inject a factory that invokes a helper process instead.
type CommandFactory func(ctx context.Context, workDir string, args ...string) *exec.Cmd

// defaultCommandFactory creates a real "agent" command.
func defaultCommandFactory(ctx context.Context, workDir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "agent", args...)
	cmd.Dir = workDir
	return cmd
}

// RunAgent spawns an agent process with the given prompt and captures its
// output. It uses "agent --model composer-1 --print --force --output-format stream-json" as the
// command line. The process is killed if ctx expires or the timeout elapses.
//
// stdout is tee'd to os.Stdout in real time for observability while also being
// captured in the returned AgentResult.
func RunAgent(ctx context.Context, workDir string, prompt string, opts ...Option) (*AgentResult, error) {
	cfg := options{
		timeout:        DefaultTimeout,
		commandFactory: defaultCommandFactory,
		stdoutWriter:   os.Stdout,
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Derive a timeout context so the process is killed on expiry.
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	model := cfg.model
	if model == "" {
		model = "composer-1"
	}
	args := []string{"--model", model, "--print", "--force", "--output-format", "stream-json", prompt}
	cmd := cfg.commandFactory(ctx, workDir, args...)

	// Capture stdout: tee to live writer + buffer.
	var stdoutBuf bytes.Buffer
	stdoutWriter := cfg.stdoutWriter
	
	// Wrap stdout writer with trace writer if trace client provided
	if cfg.traceClient != nil {
		stdoutWriter = NewTraceWriter(stdoutWriter, cfg.traceClient)
	}
	
	cmd.Stdout = io.MultiWriter(&stdoutBuf, stdoutWriter)

	// Capture stderr into a buffer.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Detect whether the process was killed due to context timeout.
	timedOut := ctx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		// Extract exit code from ExitError; otherwise treat as launch failure.
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run agent: %w", err)
		}
	}

	return &AgentResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
		TimedOut: timedOut,
	}, nil
}

// options holds optional configuration for RunAgent.
type options struct {
	timeout        time.Duration
	commandFactory CommandFactory
	stdoutWriter   io.Writer
	model          string
	traceClient    *TraceClient
}

// Option configures RunAgent behaviour.
type Option func(*options)

// WithTimeout overrides the default agent timeout.
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// WithCommandFactory injects a custom command factory (used in tests).
func WithCommandFactory(f CommandFactory) Option {
	return func(o *options) { o.commandFactory = f }
}

// WithStdoutWriter overrides the live stdout writer (default os.Stdout).
// Useful in tests to suppress or capture real-time output.
func WithStdoutWriter(w io.Writer) Option {
	return func(o *options) { o.stdoutWriter = w }
}

// WithModel overrides the default agent model.
func WithModel(model string) Option {
	return func(o *options) { o.model = model }
}

// WithTraceClient sets the trace client for emitting tool call events.
func WithTraceClient(c *TraceClient) Option {
	return func(o *options) { o.traceClient = c }
}

// RunAgentOpus runs an opus model agent for verification passes.
// Uses "agent --model claude-4.5-opus-high-thinking --print --force --output-format stream-json".
func RunAgentOpus(ctx context.Context, workDir string, prompt string, opts ...Option) (*AgentResult, error) {
	cfg := options{
		timeout:        DefaultTimeout,
		commandFactory: defaultCommandFactory,
		stdoutWriter:   os.Stdout,
		model:          "claude-4.5-opus-high-thinking",
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Derive a timeout context so the process is killed on expiry.
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	args := []string{"--model", cfg.model, "--print", "--force", "--output-format", "stream-json", prompt}
	cmd := cfg.commandFactory(ctx, workDir, args...)

	// Capture stdout: tee to live writer + buffer.
	var stdoutBuf bytes.Buffer
	stdoutWriter := cfg.stdoutWriter
	
	// Wrap stdout writer with trace writer if trace client provided
	if cfg.traceClient != nil {
		stdoutWriter = NewTraceWriter(stdoutWriter, cfg.traceClient)
	}
	
	cmd.Stdout = io.MultiWriter(&stdoutBuf, stdoutWriter)

	// Capture stderr into a buffer.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	// Detect whether the process was killed due to context timeout.
	timedOut := ctx.Err() == context.DeadlineExceeded

	exitCode := 0
	if err != nil {
		// Extract exit code from ExitError; otherwise treat as launch failure.
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run agent: %w", err)
		}
	}

	return &AgentResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
		TimedOut: timedOut,
	}, nil
}

// TraceWriter wraps an io.Writer and emits trace events for tool calls.
// It parses stream-json events and calls traceClient.StartTool/EndTool.
type TraceWriter struct {
	inner       io.Writer
	traceClient *TraceClient
	pendingSpans map[string]string // tool call ID -> span ID
	mu          sync.Mutex
	buf         []byte // Buffer for incomplete lines
}

// NewTraceWriter creates a new TraceWriter that wraps inner and emits trace events.
func NewTraceWriter(inner io.Writer, client *TraceClient) *TraceWriter {
	return &TraceWriter{
		inner:        inner,
		traceClient:  client,
		pendingSpans: make(map[string]string),
	}
}

// Write implements io.Writer. It parses JSON lines, emits trace events, and passes through to inner.
func (w *TraceWriter) Write(p []byte) (int, error) {
	// Write to inner writer first (pass through)
	n, err := w.inner.Write(p)
	if err != nil {
		return n, err
	}

	// Parse JSON lines and emit trace events
	w.mu.Lock()
	defer w.mu.Unlock()

	// Append to buffer (handle partial lines)
	w.buf = append(w.buf, p...)

	// Process complete lines
	scanner := bufio.NewScanner(strings.NewReader(string(w.buf)))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as JSON
		var event map[string]interface{}
		if err := json.Unmarshal(line, &event); err != nil {
			// Not JSON, skip
			continue
		}

		// Process tool_call events
		w.processEvent(event)
	}

	// Keep incomplete line in buffer
	// Find last newline
	lastNewline := bytes.LastIndex(w.buf, []byte("\n"))
	if lastNewline >= 0 {
		w.buf = w.buf[lastNewline+1:]
	}

	return n, nil
}

// processEvent processes a single JSON event and emits trace events if needed.
func (w *TraceWriter) processEvent(event map[string]interface{}) {
	eventType, _ := event["type"].(string)
	if eventType != "tool_call" {
		return
	}

	subtype, _ := event["subtype"].(string)
	if subtype == "" {
		return
	}

	// Extract tool call ID (if available)
	toolCallID, _ := event["id"].(string)

	switch subtype {
	case "started":
		w.handleToolStart(event, toolCallID)
	case "completed":
		w.handleToolEnd(event, toolCallID)
	}
}

// handleToolStart handles a tool_call "started" event.
func (w *TraceWriter) handleToolStart(event map[string]interface{}, toolCallID string) {
	// Extract tool name and attributes
	toolName, attrs := w.extractToolInfo(event)
	if toolName == "" {
		return
	}

	// Start tool span
	spanID := w.traceClient.StartTool(toolName, attrs)
	if spanID != "" && toolCallID != "" {
		w.pendingSpans[toolCallID] = spanID
	}
}

// handleToolEnd handles a tool_call "completed" event.
func (w *TraceWriter) handleToolEnd(event map[string]interface{}, toolCallID string) {
	// Find span ID for this tool call
	var spanID string
	if toolCallID != "" {
		spanID = w.pendingSpans[toolCallID]
		delete(w.pendingSpans, toolCallID)
	}

	if spanID == "" {
		return
	}

	// Extract result attributes
	attrs := w.extractResultAttrs(event)

	// End tool span
	w.traceClient.EndTool(spanID, attrs)
}

// extractToolInfo extracts tool name and attributes from a tool_call event.
// Returns tool name and attributes map.
func (w *TraceWriter) extractToolInfo(event map[string]interface{}) (string, map[string]string) {
	attrs := make(map[string]string)

	// Check for new nested schema: {"type":"tool_call","tool_call":{"semSearchToolCall":{...}}}
	if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		return w.extractNestedToolInfo(toolCall, attrs)
	}

	// Fall back to old schema
	name, _ := event["name"].(string)
	if name == "" {
		return "", nil
	}

	args, _ := event["arguments"].(map[string]interface{})
	if args == nil {
		return name, attrs
	}

	// Extract attributes based on tool name
	switch name {
	case "read_file":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "read", attrs
	case "write":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "write", attrs
	case "search_replace":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "edit", attrs
	case "run_terminal_cmd":
		if cmd, ok := args["command"].(string); ok {
			attrs["command"] = cmd
		}
		return "shell", attrs
	case "grep":
		if pattern, ok := args["pattern"].(string); ok {
			attrs["pattern"] = pattern
		}
		return "grep", attrs
	case "codebase_search":
		if query, ok := args["query"].(string); ok {
			attrs["query"] = query
		}
		return "search", attrs
	}

	return name, attrs
}

// extractNestedToolInfo extracts tool info from nested schema.
func (w *TraceWriter) extractNestedToolInfo(toolCall map[string]interface{}, attrs map[string]string) (string, map[string]string) {
	// Check for semantic search
	if sem, ok := toolCall["semSearchToolCall"].(map[string]interface{}); ok {
		if args, ok := sem["args"].(map[string]interface{}); ok {
			if query, ok := args["query"].(string); ok {
				attrs["query"] = query
			}
		}
		return "search", attrs
	}

	// Check for edit
	if edit, ok := toolCall["editToolCall"].(map[string]interface{}); ok {
		if args, ok := edit["args"].(map[string]interface{}); ok {
			if path, ok := args["path"].(string); ok {
				attrs["file_path"] = path
			}
		}
		return "edit", attrs
	}

	// Check for read
	if read, ok := toolCall["readToolCall"].(map[string]interface{}); ok {
		if args, ok := read["args"].(map[string]interface{}); ok {
			if path, ok := args["target_file"].(string); ok {
				attrs["file_path"] = path
			}
		}
		return "read", attrs
	}

	// Check for grep
	if grep, ok := toolCall["grepToolCall"].(map[string]interface{}); ok {
		if args, ok := grep["args"].(map[string]interface{}); ok {
			if pattern, ok := args["pattern"].(string); ok {
				attrs["pattern"] = pattern
			}
		}
		return "grep", attrs
	}

	// Check for shell
	if shell, ok := toolCall["shellToolCall"].(map[string]interface{}); ok {
		if args, ok := shell["args"].(map[string]interface{}); ok {
			if cmd, ok := args["command"].(string); ok {
				attrs["command"] = cmd
			}
		}
		return "shell", attrs
	}

	return "", attrs
}

// extractResultAttrs extracts attributes from a tool_call "completed" event result.
func (w *TraceWriter) extractResultAttrs(event map[string]interface{}) map[string]string {
	attrs := make(map[string]string)

	// Check for result in nested schema
	if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		// Check for result in any tool type
		for _, toolData := range toolCall {
			if tool, ok := toolData.(map[string]interface{}); ok {
				if result, ok := tool["result"].(map[string]interface{}); ok {
					// Extract exit code for shell commands
					if exitCode, ok := result["exit_code"].(float64); ok {
						attrs["exit_code"] = fmt.Sprintf("%.0f", exitCode)
					}
					// Extract lines changed for edit operations (if available)
					if linesChanged, ok := result["lines_changed"].(float64); ok {
						attrs["lines_changed"] = fmt.Sprintf("%.0f", linesChanged)
					}
				}
			}
		}
	}

	// Check for result in old schema
	if result, ok := event["result"].(map[string]interface{}); ok {
		if exitCode, ok := result["exit_code"].(float64); ok {
			attrs["exit_code"] = fmt.Sprintf("%.0f", exitCode)
		}
	}

	return attrs
}
