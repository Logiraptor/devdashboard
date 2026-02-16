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
	"time"
)

// ToolEvent represents a tool call event from the agent's stream-json output.
type ToolEvent struct {
	Name       string            // Tool name (e.g., "read", "write", "grep")
	Started    bool              // true for "started", false for "ended"
	DurationMs int64             // Duration in milliseconds (for ended events)
	Attributes map[string]string // Extracted attributes (e.g., file_path, query)
}

// ToolEventObserver receives tool call events as they are parsed from the agent output.
type ToolEventObserver interface {
	OnToolStart(event ToolEvent)
	OnToolEnd(event ToolEvent)
}

// toolEventWriter wraps an io.Writer and parses tool_call JSON lines,
// calling the observer for each tool event found.
type toolEventWriter struct {
	inner    io.Writer
	observer ToolEventObserver
	buffer   []byte // Buffer for partial lines
}

// DefaultTimeout is the per-agent execution timeout.
// Set to 10 minutes to allow complex tasks while preventing hung agents.
// Most agent runs complete in 1-5 minutes.
const DefaultTimeout = 10 * time.Minute

// AgentResult holds the outcome of a single agent invocation.
type AgentResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	TimedOut bool // true if the agent was killed due to timeout

	// ChatID is the chat session ID from the agent, extracted from the result event.
	// Useful for debugging failed runs by finding the chat in Cursor.
	ChatID string

	// ErrorMessage is the error message from the agent's result event, if any.
	ErrorMessage string
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

// runAgentInternal is the shared implementation for running an agent process.
// It applies options, creates a timeout context, builds command arguments,
// captures stdout/stderr, runs the command, and returns the result.
func runAgentInternal(ctx context.Context, workDir, prompt, defaultModel string, opts ...Option) (*AgentResult, error) {
	cfg := options{
		timeout:        DefaultTimeout,
		commandFactory: defaultCommandFactory,
		stdoutWriter:   os.Stdout,
		model:          defaultModel,
	}
	for _, o := range opts {
		o(&cfg)
	}

	// Derive a timeout context so the process is killed on expiry.
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	model := cfg.model
	if model == "" {
		model = defaultModel
	}
	args := []string{"--model", model, "--print", "--force", "--output-format", "stream-json", prompt}
	cmd := cfg.commandFactory(ctx, workDir, args...)

	// Capture stdout: tee to live writer + buffer.
	var stdoutBuf bytes.Buffer
	stdoutWriter := cfg.stdoutWriter
	
	// Wrap with toolEventWriter if observer is provided
	if cfg.toolEventObserver != nil {
		stdoutWriter = &toolEventWriter{
			inner:    cfg.stdoutWriter,
			observer: cfg.toolEventObserver,
		}
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

	result := &AgentResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
		TimedOut: timedOut,
	}

	// Parse chatId and error from the agent's stdout (stream-json format)
	result.ChatID, result.ErrorMessage = parseAgentResultEvent(stdoutBuf.String())

	return result, nil
}

// RunAgent spawns an agent process with the given prompt and captures its
// output. It uses "agent --model composer-1 --print --force --output-format stream-json" as the
// command line. The process is killed if ctx expires or the timeout elapses.
//
// stdout is tee'd to os.Stdout in real time for observability while also being
// captured in the returned AgentResult.
func RunAgent(ctx context.Context, workDir string, prompt string, opts ...Option) (*AgentResult, error) {
	return runAgentInternal(ctx, workDir, prompt, "composer-1", opts...)
}

// options holds optional configuration for RunAgent.
type options struct {
	timeout          time.Duration
	commandFactory   CommandFactory
	stdoutWriter     io.Writer
	model            string
	toolEventObserver ToolEventObserver
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

// WithToolEventObserver injects a tool event observer for parsing tool_call events.
func WithToolEventObserver(observer ToolEventObserver) Option {
	return func(o *options) {
		o.toolEventObserver = observer
	}
}

// Write implements io.Writer by buffering partial lines and parsing complete JSON lines.
func (w *toolEventWriter) Write(p []byte) (int, error) {
	// Write to inner writer first
	n, err := w.inner.Write(p)
	if err != nil {
		return n, err
	}

	// Append to buffer
	w.buffer = append(w.buffer, p...)

	// Process complete lines (lines ending with newline)
	for {
		newlineIdx := bytes.IndexByte(w.buffer, '\n')
		if newlineIdx == -1 {
			// No complete line yet, keep everything in buffer
			break
		}

		// Extract complete line (including newline)
		line := w.buffer[:newlineIdx+1]
		// Parse the line (without the newline)
		w.parseToolEventLine(line[:len(line)-1])

		// Remove processed line from buffer
		w.buffer = w.buffer[newlineIdx+1:]
	}

	return n, nil
}

// parseToolEventLine parses a single JSON line and calls observer if it's a tool_call event.
func (w *toolEventWriter) parseToolEventLine(line []byte) {
	if len(line) == 0 {
		return
	}

	var event map[string]interface{}
	if err := json.Unmarshal(line, &event); err != nil {
		return
	}

	eventType, _ := event["type"].(string)
	if eventType != "tool_call" {
		return
	}

	subtype, _ := event["subtype"].(string)
	if subtype != "started" && subtype != "ended" {
		return
	}

	// Extract tool name
	var toolName string
	if name, ok := event["name"].(string); ok {
		toolName = name
	} else if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		// Try to extract name from nested tool_call object
		for key := range toolCall {
			toolName = key
			break // Use first key as tool name
		}
	}

	if toolName == "" {
		return
	}

	// Extract attributes
	attrs := make(map[string]string)
	if arguments, ok := event["arguments"].(map[string]interface{}); ok {
		for k, v := range arguments {
			if s, ok := v.(string); ok {
				attrs[k] = s
			} else if s := fmt.Sprintf("%v", v); s != "" {
				attrs[k] = s
			}
		}
	}

	// Extract duration for ended events
	var durationMs int64
	if duration, ok := event["duration_ms"].(float64); ok {
		durationMs = int64(duration)
	}

	toolEvent := ToolEvent{
		Name:       toolName,
		Started:    subtype == "started",
		DurationMs: durationMs,
		Attributes: attrs,
	}

	if toolEvent.Started {
		w.observer.OnToolStart(toolEvent)
	} else {
		w.observer.OnToolEnd(toolEvent)
	}
}

// RunAgentOpus runs an opus model agent for verification passes.
// Uses "agent --model claude-4.5-opus-high-thinking --print --force --output-format stream-json".
func RunAgentOpus(ctx context.Context, workDir string, prompt string, opts ...Option) (*AgentResult, error) {
	return runAgentInternal(ctx, workDir, prompt, "claude-4.5-opus-high-thinking", opts...)
}

// parseAgentResultEvent parses the agent's stdout for the final "result" event
// and extracts chatId and error message.
// The agent outputs JSON lines, and the result event has the format:
// {"type":"result","chatId":"...","error":"...","duration_ms":...}
func parseAgentResultEvent(stdout string) (chatID, errorMsg string) {
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		if eventType != "result" {
			continue
		}

		// Extract chatId - may be at top level or nested
		if id, ok := event["chatId"].(string); ok {
			chatID = id
		} else if id, ok := event["chat_id"].(string); ok {
			chatID = id
		}

		// Extract error message
		if errStr, ok := event["error"].(string); ok && errStr != "" {
			errorMsg = errStr
		} else if errObj, ok := event["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				errorMsg = msg
			}
		}
	}
	return chatID, errorMsg
}
