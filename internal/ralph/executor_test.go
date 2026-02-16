package ralph

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"devdeploy/internal/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test-helper process
// ---------------------------------------------------------------------------
//
// Tests use the "TestHelperProcess" pattern: re-exec the test binary with a
// sentinel env var so the child behaves as a fake agent. This lets us test
// the plumbing (exit codes, stdout/stderr capture, timeouts) without an
// actual agent binary.

func TestHelperProcess(t *testing.T) {
	if os.Getenv("DD_TEST_HELPER") != "1" {
		return // not the helper invocation
	}
	// Dispatch on DD_TEST_MODE.
	switch os.Getenv("DD_TEST_MODE") {
	case "echo":
		// Echo args after "--" to stdout, nothing to stderr.
		args := os.Args[1:]
		for i, a := range args {
			if a == "--" {
				args = args[i+1:]
				break
			}
		}
		for i, a := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(a)
		}
	case "stderr":
		fmt.Fprint(os.Stderr, "agent error output")
	case "exit":
		code, _ := strconv.Atoi(os.Getenv("DD_EXIT_CODE"))
		os.Exit(code)
	case "slow":
		// Sleep longer than the test timeout to trigger kill.
		time.Sleep(30 * time.Second)
	case "tool_calls":
		// Output tool_call JSON lines
		fmt.Println(`{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"test.go"}}`)
		fmt.Println(`{"type":"tool_call","subtype":"ended","name":"read","duration_ms":42}`)
	default:
		fmt.Fprintln(os.Stderr, "unknown DD_TEST_MODE")
		os.Exit(2)
	}
	os.Exit(0)
}

// helperFactory returns a CommandFactory that re-invokes the current test
// binary as the helper process.
func helperFactory(mode string, envExtra ...string) CommandFactory {
	return func(ctx context.Context, workDir string, args ...string) *exec.Cmd {
		// Build a command that re-executes "go test" in helper mode.
		cs := append([]string{"-test.run=^TestHelperProcess$", "--"}, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(),
			"DD_TEST_HELPER=1",
			"DD_TEST_MODE="+mode,
		)
		cmd.Env = append(cmd.Env, envExtra...)
		return cmd
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRunAgent_CapturesStdout(t *testing.T) {
	var live bytes.Buffer
	result, err := RunAgent(
		context.Background(),
		t.TempDir(),
		"hello world",
		WithCommandFactory(helperFactory("echo")),
		WithStdoutWriter(&live),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
	// The echo helper prints the args (--model composer-1 --print --force --output-format stream-json hello world).
	want := "--model composer-1 --print --force --output-format stream-json hello world"
	if result.Stdout != want {
		t.Errorf("stdout = %q, want %q", result.Stdout, want)
	}
	// Live writer should have received the same content.
	if live.String() != want {
		t.Errorf("live writer = %q, want %q", live.String(), want)
	}
	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestRunAgent_CapturesStderr(t *testing.T) {
	var live bytes.Buffer
	result, err := RunAgent(
		context.Background(),
		t.TempDir(),
		"test",
		WithCommandFactory(helperFactory("stderr")),
		WithStdoutWriter(&live),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stderr != "agent error output" {
		t.Errorf("stderr = %q, want %q", result.Stderr, "agent error output")
	}
}

func TestRunAgent_NonZeroExit(t *testing.T) {
	var live bytes.Buffer
	result, err := RunAgent(
		context.Background(),
		t.TempDir(),
		"test",
		WithCommandFactory(helperFactory("exit", "DD_EXIT_CODE=42")),
		WithStdoutWriter(&live),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}
}

func TestRunAgent_TimeoutKillsProcess(t *testing.T) {
	var live bytes.Buffer
	start := time.Now()
	result, err := RunAgent(
		context.Background(),
		t.TempDir(),
		"test",
		WithCommandFactory(helperFactory("slow")),
		WithStdoutWriter(&live),
		WithTimeout(200*time.Millisecond),
	)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Process should have been killed, yielding a non-zero exit code.
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code after timeout kill")
	}
	// Should complete well under 5s (the helper sleeps 30s).
	if elapsed > 3*time.Second {
		t.Errorf("timeout did not kill process promptly (elapsed %v)", elapsed)
	}
}

func TestRunAgent_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	var live bytes.Buffer
	result, err := RunAgent(
		ctx,
		t.TempDir(),
		"test",
		WithCommandFactory(helperFactory("slow")),
		WithStdoutWriter(&live),
		WithTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code after context cancellation")
	}
}

func TestRunAgent_InvalidWorkDir(t *testing.T) {
	var live bytes.Buffer
	_, err := RunAgent(
		context.Background(),
		"/nonexistent/path/that/should/not/exist",
		"test",
		WithCommandFactory(helperFactory("echo")),
		WithStdoutWriter(&live),
		WithTimeout(5*time.Second),
	)
	if err == nil {
		t.Fatal("expected error for invalid work dir")
	}
}

// ---------------------------------------------------------------------------
// parseAgentResultEvent tests
// ---------------------------------------------------------------------------

func TestParseAgentResultEvent_ChatID(t *testing.T) {
	stdout := `{"type":"system","content":"system prompt"}
{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}
{"type":"result","chatId":"chat-abc123","duration_ms":5000}
`
	chatID, errMsg := parseAgentResultEvent(stdout)
	if chatID != "chat-abc123" {
		t.Errorf("chatID = %q, want %q", chatID, "chat-abc123")
	}
	if errMsg != "" {
		t.Errorf("errorMsg = %q, want empty", errMsg)
	}
}

func TestParseAgentResultEvent_ChatIDSnakeCase(t *testing.T) {
	stdout := `{"type":"result","chat_id":"chat-xyz789","duration_ms":3000}
`
	chatID, errMsg := parseAgentResultEvent(stdout)
	if chatID != "chat-xyz789" {
		t.Errorf("chatID = %q, want %q", chatID, "chat-xyz789")
	}
	if errMsg != "" {
		t.Errorf("errorMsg = %q, want empty", errMsg)
	}
}

func TestParseAgentResultEvent_ErrorString(t *testing.T) {
	stdout := `{"type":"result","chatId":"chat-err001","error":"Agent crashed unexpectedly"}
`
	chatID, errMsg := parseAgentResultEvent(stdout)
	if chatID != "chat-err001" {
		t.Errorf("chatID = %q, want %q", chatID, "chat-err001")
	}
	if errMsg != "Agent crashed unexpectedly" {
		t.Errorf("errorMsg = %q, want %q", errMsg, "Agent crashed unexpectedly")
	}
}

func TestParseAgentResultEvent_ErrorObject(t *testing.T) {
	stdout := `{"type":"result","chatId":"chat-err002","error":{"message":"Detailed error message","code":500}}
`
	chatID, errMsg := parseAgentResultEvent(stdout)
	if chatID != "chat-err002" {
		t.Errorf("chatID = %q, want %q", chatID, "chat-err002")
	}
	if errMsg != "Detailed error message" {
		t.Errorf("errorMsg = %q, want %q", errMsg, "Detailed error message")
	}
}

func TestParseAgentResultEvent_NoResult(t *testing.T) {
	stdout := `{"type":"system","content":"system prompt"}
{"type":"tool_call","name":"read"}
`
	chatID, errMsg := parseAgentResultEvent(stdout)
	if chatID != "" {
		t.Errorf("chatID = %q, want empty", chatID)
	}
	if errMsg != "" {
		t.Errorf("errorMsg = %q, want empty", errMsg)
	}
}

func TestParseAgentResultEvent_EmptyStdout(t *testing.T) {
	chatID, errMsg := parseAgentResultEvent("")
	if chatID != "" {
		t.Errorf("chatID = %q, want empty", chatID)
	}
	if errMsg != "" {
		t.Errorf("errorMsg = %q, want empty", errMsg)
	}
}

func TestParseAgentResultEvent_InvalidJSON(t *testing.T) {
	stdout := `not json
{"type":"result","chatId":"chat-123"}
more garbage`
	chatID, errMsg := parseAgentResultEvent(stdout)
	// Should still extract from the valid line
	if chatID != "chat-123" {
		t.Errorf("chatID = %q, want %q", chatID, "chat-123")
	}
	if errMsg != "" {
		t.Errorf("errorMsg = %q, want empty", errMsg)
	}
}

// ---------------------------------------------------------------------------
// toolEventWriter tests
// ---------------------------------------------------------------------------

// mockObserver implements ToolEventObserver for testing
type mockObserver struct {
	onToolStart func(ToolEvent)
	onToolEnd   func(ToolEvent)
}

func (m *mockObserver) OnToolStart(event ToolEvent) {
	if m.onToolStart != nil {
		m.onToolStart(event)
	}
}

func (m *mockObserver) OnToolEnd(event ToolEvent) {
	if m.onToolEnd != nil {
		m.onToolEnd(event)
	}
}

func TestToolEventWriter_ParsesToolCalls(t *testing.T) {
	var events []ToolEvent
	obs := &mockObserver{
		onToolStart: func(e ToolEvent) { events = append(events, e) },
		onToolEnd:   func(e ToolEvent) { events = append(events, e) },
	}

	w := &toolEventWriter{inner: io.Discard, observer: obs}

	// Write tool_call started
	w.Write([]byte(`{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}` + "\n"))

	// Write tool_call ended
	w.Write([]byte(`{"type":"tool_call","subtype":"ended","name":"read","duration_ms":100}` + "\n"))

	// Verify events
	require.Len(t, events, 2)
	assert.Equal(t, "read", events[0].Name)
	assert.True(t, events[0].Started)
	assert.Equal(t, "foo.go", events[0].Attributes["path"])
	assert.Equal(t, "read", events[1].Name)
	assert.False(t, events[1].Started)
	assert.Equal(t, int64(100), events[1].DurationMs)
}

func TestToolEventWriter_PartialLineHandling(t *testing.T) {
	var events []ToolEvent
	obs := &mockObserver{
		onToolStart: func(e ToolEvent) { events = append(events, e) },
		onToolEnd:   func(e ToolEvent) { events = append(events, e) },
	}

	w := &toolEventWriter{inner: io.Discard, observer: obs}

	// Write partial line (split across JSON boundary)
	line1 := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}`
	line2 := `}` + "\n"
	
	w.Write([]byte(line1))
	require.Len(t, events, 0) // No events yet, line incomplete

	w.Write([]byte(line2))
	require.Len(t, events, 1) // Now we have the complete event
	assert.Equal(t, "read", events[0].Name)
	assert.True(t, events[0].Started)

	// Write another partial line split differently
	events = events[:0]
	part1 := `{"type":"tool_call","subtype":"ended","name":"write","duration_ms":50`
	part2 := `}` + "\n"
	
	w.Write([]byte(part1))
	require.Len(t, events, 0) // No events yet

	w.Write([]byte(part2))
	require.Len(t, events, 1) // Complete event
	assert.Equal(t, "write", events[0].Name)
	assert.False(t, events[0].Started)
	assert.Equal(t, int64(50), events[0].DurationMs)
}

func TestToolEventWriter_IntegrationWithMockAgent(t *testing.T) {
	var events []ToolEvent
	obs := &mockObserver{
		onToolStart: func(e ToolEvent) { events = append(events, e) },
		onToolEnd:   func(e ToolEvent) { events = append(events, e) },
	}

	var live bytes.Buffer
	result, err := RunAgent(
		context.Background(),
		t.TempDir(),
		"test",
		WithCommandFactory(helperFactory("tool_calls")),
		WithStdoutWriter(&live),
		WithToolEventObserver(obs),
		WithTimeout(5*time.Second),
	)

	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)
	require.Len(t, events, 2)
	assert.Equal(t, "read", events[0].Name)
	assert.True(t, events[0].Started)
	assert.Equal(t, "test.go", events[0].Attributes["path"])
	assert.Equal(t, "read", events[1].Name)
	assert.False(t, events[1].Started)
	assert.Equal(t, int64(42), events[1].DurationMs)
}

func TestToolEventWriter_TraceManagerReceivesToolSpans(t *testing.T) {
	// Create trace manager directly (avoiding import cycle with tui)
	manager := trace.NewManager(10)
	
	// Start a loop to establish trace context
	traceID := trace.NewTraceID()
	loopSpanID := trace.NewSpanID()
	loopStartEvent := trace.TraceEvent{
		TraceID:   traceID,
		SpanID:   loopSpanID,
		Type:     trace.EventLoopStart,
		Name:     "ralph-loop",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"model":          "composer-1",
			"epic":           "test-epic",
			"workdir":        t.TempDir(),
			"max_iterations": "1",
		},
	}
	manager.HandleEvent(loopStartEvent)
	
	// Start an iteration to set parent context
	iterSpanID := trace.NewSpanID()
	iterStartEvent := trace.TraceEvent{
		TraceID:   traceID,
		SpanID:   iterSpanID,
		ParentID: loopSpanID,
		Type:     trace.EventIterationStart,
		Name:     "iteration-1",
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"bead_id":    "bead-123",
			"bead_title": "Test Bead",
			"iteration":  "1",
		},
	}
	manager.HandleEvent(iterStartEvent)
	
	// Create observer that forwards tool events to trace manager
	var toolSpans []string
	obs := &mockObserver{
		onToolStart: func(e ToolEvent) {
			attrs := make(map[string]string)
			for k, v := range e.Attributes {
				attrs[k] = v
			}
			spanID := trace.NewSpanID()
			toolSpans = append(toolSpans, spanID)
			toolStartEvent := trace.TraceEvent{
				TraceID:    traceID,
				SpanID:     spanID,
				ParentID:   iterSpanID,
				Type:       trace.EventToolStart,
				Name:       e.Name,
				Timestamp:  time.Now(),
				Attributes: attrs,
			}
			manager.HandleEvent(toolStartEvent)
		},
		onToolEnd: func(e ToolEvent) {
			if len(toolSpans) > 0 {
				spanID := toolSpans[len(toolSpans)-1]
				attrs := make(map[string]string)
				if e.DurationMs > 0 {
					attrs["duration_ms"] = fmt.Sprintf("%d", e.DurationMs)
				}
				toolEndEvent := trace.TraceEvent{
					TraceID:    traceID,
					SpanID:     spanID,
					ParentID:   iterSpanID,
					Type:       trace.EventToolEnd,
					Name:       "tool-end",
					Timestamp:  time.Now(),
					Attributes: attrs,
				}
				manager.HandleEvent(toolEndEvent)
			}
		},
	}

	// Create writer with observer
	var live bytes.Buffer
	w := &toolEventWriter{
		inner:    &live,
		observer: obs,
	}

	// Write tool events
	w.Write([]byte(`{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}` + "\n"))
	time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
	w.Write([]byte(`{"type":"tool_call","subtype":"ended","name":"read","duration_ms":100}` + "\n"))
	w.Write([]byte(`{"type":"tool_call","subtype":"started","name":"write","arguments":{"path":"bar.go"}}` + "\n"))
	time.Sleep(10 * time.Millisecond)
	w.Write([]byte(`{"type":"tool_call","subtype":"ended","name":"write","duration_ms":200}` + "\n"))

	// Verify spans were created
	require.Len(t, toolSpans, 2)
	
	// Verify trace structure
	activeTrace := manager.GetActiveTrace()
	require.NotNil(t, activeTrace)
	require.NotNil(t, activeTrace.RootSpan)
	
	// Find iteration span
	var iterSpan *trace.Span
	var findIterSpan func(*trace.Span) *trace.Span
	findIterSpan = func(s *trace.Span) *trace.Span {
		if s.SpanID == iterSpanID {
			return s
		}
		for _, child := range s.Children {
			if found := findIterSpan(child); found != nil {
				return found
			}
		}
		return nil
	}
	iterSpan = findIterSpan(activeTrace.RootSpan)
	require.NotNil(t, iterSpan, "iteration span should exist")
	
	// Verify tool spans are children of iteration span
	require.Len(t, iterSpan.Children, 2, "iteration should have 2 tool children")
	
	// Verify first tool span (read)
	readSpan := iterSpan.Children[0]
	assert.Equal(t, "read", readSpan.Name)
	assert.Equal(t, toolSpans[0], readSpan.SpanID)
	assert.Equal(t, iterSpanID, readSpan.ParentID)
	assert.Equal(t, "foo.go", readSpan.Attributes["path"])
	assert.Greater(t, readSpan.Duration, time.Duration(0), "read span should have duration")
	
	// Verify second tool span (write)
	writeSpan := iterSpan.Children[1]
	assert.Equal(t, "write", writeSpan.Name)
	assert.Equal(t, toolSpans[1], writeSpan.SpanID)
	assert.Equal(t, iterSpanID, writeSpan.ParentID)
	assert.Equal(t, "bar.go", writeSpan.Attributes["path"])
	assert.Greater(t, writeSpan.Duration, time.Duration(0), "write span should have duration")
}
