package ralph

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
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

type testToolObserver struct {
	starts []struct {
		name      string
		arguments map[string]interface{}
	}
	ends []struct {
		name       string
		arguments  map[string]interface{}
		durationMs int64
	}
}

func (o *testToolObserver) OnToolStart(name string, arguments map[string]interface{}) {
	o.starts = append(o.starts, struct {
		name      string
		arguments map[string]interface{}
	}{name: name, arguments: arguments})
}

func (o *testToolObserver) OnToolEnd(name string, arguments map[string]interface{}, durationMs int64) {
	o.ends = append(o.ends, struct {
		name       string
		arguments  map[string]interface{}
		durationMs int64
	}{name: name, arguments: arguments, durationMs: durationMs})
}

func TestToolEventWriter_ToolStart(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"tool_call","subtype":"started","name":"read_file","arguments":{"path":"foo.go"}}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(observer.starts) != 1 {
		t.Fatalf("expected 1 start event, got %d", len(observer.starts))
	}
	if observer.starts[0].name != "read_file" {
		t.Errorf("name = %q, want %q", observer.starts[0].name, "read_file")
	}
	if path, ok := observer.starts[0].arguments["path"].(string); !ok || path != "foo.go" {
		t.Errorf("arguments[path] = %v, want %q", observer.starts[0].arguments["path"], "foo.go")
	}
	if len(observer.ends) != 0 {
		t.Errorf("expected 0 end events, got %d", len(observer.ends))
	}
	if inner.String() != input {
		t.Errorf("inner writer = %q, want %q", inner.String(), input)
	}
}

func TestToolEventWriter_ToolEnd(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"tool_call","subtype":"ended","name":"read_file","arguments":{"path":"foo.go"},"duration_ms":150}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(observer.ends) != 1 {
		t.Fatalf("expected 1 end event, got %d", len(observer.ends))
	}
	if observer.ends[0].name != "read_file" {
		t.Errorf("name = %q, want %q", observer.ends[0].name, "read_file")
	}
	if observer.ends[0].durationMs != 150 {
		t.Errorf("durationMs = %d, want %d", observer.ends[0].durationMs, 150)
	}
	if len(observer.starts) != 0 {
		t.Errorf("expected 0 start events, got %d", len(observer.starts))
	}
	if inner.String() != input {
		t.Errorf("inner writer = %q, want %q", inner.String(), input)
	}
}

func TestToolEventWriter_PartialLines(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	// Write partial line
	_, err := writer.Write([]byte(`{"type":"tool_call","subtype":"started","name":"read_file","arguments":{"path":"foo.go"}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No events yet (no newline)
	if len(observer.starts) != 0 {
		t.Errorf("expected 0 start events before newline, got %d", len(observer.starts))
	}

	// Complete the line
	_, err = writer.Write([]byte("\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now should have the event
	if len(observer.starts) != 1 {
		t.Fatalf("expected 1 start event after newline, got %d", len(observer.starts))
	}
}

func TestToolEventWriter_MultipleLines(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"tool_call","subtype":"started","name":"read_file","arguments":{"path":"foo.go"}}
{"type":"tool_call","subtype":"ended","name":"read_file","arguments":{"path":"foo.go"},"duration_ms":150}
{"type":"tool_call","subtype":"started","name":"edit_file","arguments":{"path":"bar.go"}}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(observer.starts) != 2 {
		t.Errorf("expected 2 start events, got %d", len(observer.starts))
	}
	if len(observer.ends) != 1 {
		t.Errorf("expected 1 end event, got %d", len(observer.ends))
	}
}

func TestToolEventWriter_NonToolCallEvents(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"system","content":"system prompt"}
{"type":"result","chatId":"chat-123"}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should ignore non-tool_call events
	if len(observer.starts) != 0 {
		t.Errorf("expected 0 start events, got %d", len(observer.starts))
	}
	if len(observer.ends) != 0 {
		t.Errorf("expected 0 end events, got %d", len(observer.ends))
	}
}

func TestToolEventWriter_InvalidJSON(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `not json
{"type":"tool_call","subtype":"started","name":"read_file","arguments":{"path":"foo.go"}}
more garbage
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should parse the valid JSON line and ignore invalid ones
	if len(observer.starts) != 1 {
		t.Errorf("expected 1 start event, got %d", len(observer.starts))
	}
}

func TestToolEventWriter_NoObserver(t *testing.T) {
	var inner bytes.Buffer
	writer := newToolEventWriter(&inner, nil)

	input := `{"type":"tool_call","subtype":"started","name":"read_file","arguments":{"path":"foo.go"}}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic with nil observer
	if inner.String() != input {
		t.Errorf("inner writer = %q, want %q", inner.String(), input)
	}
}

func TestToolEventWriter_NonStartedOrEndedSubtype(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"tool_call","subtype":"other","name":"read_file","arguments":{"path":"foo.go"}}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should ignore subtypes other than "started" or "ended"
	if len(observer.starts) != 0 {
		t.Errorf("expected 0 start events, got %d", len(observer.starts))
	}
	if len(observer.ends) != 0 {
		t.Errorf("expected 0 end events, got %d", len(observer.ends))
	}
}

func TestToolEventWriter_MissingName(t *testing.T) {
	var inner bytes.Buffer
	observer := &testToolObserver{}
	writer := newToolEventWriter(&inner, observer)

	input := `{"type":"tool_call","subtype":"started","arguments":{"path":"foo.go"}}
`
	_, err := writer.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should ignore events without name
	if len(observer.starts) != 0 {
		t.Errorf("expected 0 start events, got %d", len(observer.starts))
	}
}
