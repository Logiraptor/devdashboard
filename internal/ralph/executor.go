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
	cmd.Stdout = io.MultiWriter(&stdoutBuf, cfg.stdoutWriter)

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
	timeout        time.Duration
	commandFactory CommandFactory
	stdoutWriter   io.Writer
	model          string
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

// RunAgentOpus runs an opus model agent for verification passes.
// Uses "agent --model claude-4.5-opus-high-thinking --print --force --output-format stream-json".
func RunAgentOpus(ctx context.Context, workDir string, prompt string, opts ...Option) (*AgentResult, error) {
	return runAgentInternal(ctx, workDir, prompt, "claude-4.5-opus-high-thinking", opts...)
}

// ToolEventObserver receives notifications about tool call events from agent stdout.
// All methods are optional — implement only what you need.
type ToolEventObserver interface {
	// OnToolStart is called when a tool call starts.
	OnToolStart(name string, arguments map[string]interface{})

	// OnToolEnd is called when a tool call completes.
	OnToolEnd(name string, arguments map[string]interface{}, durationMs int64)
}

// NoopToolEventObserver is a ToolEventObserver that does nothing.
// Embed this in your observer to avoid implementing unused methods.
type NoopToolEventObserver struct{}

func (NoopToolEventObserver) OnToolStart(string, map[string]interface{}) {}
func (NoopToolEventObserver) OnToolEnd(string, map[string]interface{}, int64) {}

// ToolEvent represents a parsed tool call event from agent stdout.
type ToolEvent struct {
	Name       string
	Subtype    string // "started" or "ended"
	Arguments  map[string]interface{}
	DurationMs int64 // Only present for "ended" events
}

// toolEventWriter wraps a writer and parses tool events from JSON lines.
// It accumulates partial lines in a buffer and parses complete JSON lines
// to extract tool_call events, calling observer methods as events are detected.
type toolEventWriter struct {
	inner    io.Writer
	observer ToolEventObserver
	buf      bytes.Buffer // accumulates partial lines
}

// newToolEventWriter creates a new toolEventWriter that wraps the given writer
// and calls the observer for tool events.
func newToolEventWriter(inner io.Writer, observer ToolEventObserver) *toolEventWriter {
	return &toolEventWriter{
		inner:    inner,
		observer: observer,
	}
}

// Write writes data to the inner writer and parses complete JSON lines
// to extract tool_call events. Partial lines are buffered until a newline
// is received.
func (w *toolEventWriter) Write(p []byte) (n int, err error) {
	// Write to inner writer first
	n, err = w.inner.Write(p)
	if err != nil {
		return n, err
	}

	// Accumulate in buffer
	w.buf.Write(p)

	// Process complete lines
	for {
		// Find the first newline in the buffer
		bufBytes := w.buf.Bytes()
		newlineIdx := bytes.IndexByte(bufBytes, '\n')
		if newlineIdx == -1 {
			// No complete line yet
			break
		}

		// Extract the line (including newline)
		lineBytes := make([]byte, newlineIdx+1)
		copy(lineBytes, bufBytes[:newlineIdx+1])
		w.buf.Next(newlineIdx + 1)

		// Remove newline
		line := strings.TrimSuffix(string(lineBytes), "\n")
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON or invalid JSON - ignore gracefully
			continue
		}

		// Check if it's a tool_call event
		eventType, _ := event["type"].(string)
		if eventType != "tool_call" {
			continue
		}

		// Extract subtype
		subtype, _ := event["subtype"].(string)
		if subtype != "started" && subtype != "ended" {
			continue
		}

		// Extract tool name
		name, _ := event["name"].(string)
		if name == "" {
			continue
		}

		// Extract arguments
		arguments := make(map[string]interface{})
		if args, ok := event["arguments"].(map[string]interface{}); ok {
			arguments = args
		}

		// Extract duration_ms for ended events
		var durationMs int64
		if subtype == "ended" {
			if dur, ok := event["duration_ms"].(float64); ok {
				durationMs = int64(dur)
			}
		}

		// Call observer
		if w.observer != nil {
			if subtype == "started" {
				w.observer.OnToolStart(name, arguments)
			} else if subtype == "ended" {
				w.observer.OnToolEnd(name, arguments, durationMs)
			}
		}
	}

	return n, nil
}

// toolEventWriter wraps a writer and parses tool events from JSON lines.
// It accumulates partial lines in a buffer and calls observer methods
// when tool_call events are detected.
type toolEventWriter struct {
	inner    io.Writer
	observer ProgressObserver
	buf      bytes.Buffer // accumulates partial lines
}

// NewToolEventWriter creates a new toolEventWriter that wraps the given writer
// and calls observer methods for tool events parsed from JSON lines.
func NewToolEventWriter(inner io.Writer, observer ProgressObserver) io.Writer {
	if observer == nil {
		return inner // no observer, no need to wrap
	}
	return &toolEventWriter{
		inner:    inner,
		observer: observer,
	}
}

// Write writes data to the inner writer and parses complete JSON lines
// to detect tool_call events. Partial lines are buffered until a newline
// is encountered.
func (w *toolEventWriter) Write(p []byte) (n int, err error) {
	// Write to inner writer first
	n, err = w.inner.Write(p)
	if err != nil {
		return n, err
	}

	// Append to buffer
	w.buf.Write(p)

	// Process complete lines
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// No complete line yet, put it back
			w.buf.WriteString(line)
			break
		}

		// Remove trailing newline
		line = strings.TrimSuffix(line, "\n")

		// Try to parse as tool event
		event := ParseToolEvent(line)
		if event != nil {
			if event.Started {
				w.observer.OnToolStart(*event)
			} else {
				w.observer.OnToolEnd(*event)
			}
		}
		// Ignore non-JSON lines and non-tool_call events gracefully
	}

	return n, nil
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
