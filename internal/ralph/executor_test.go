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

	"devdeploy/internal/beads"
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
<<<<<<< HEAD

// ---------------------------------------------------------------------------
// ParseToolEvent tests
// ---------------------------------------------------------------------------

func TestParseToolEvent_Started(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Name != "read" {
		t.Errorf("Name = %q, want %q", event.Name, "read")
	}
	if !event.Started {
		t.Error("Started = false, want true")
	}
	if event.Attributes["file_path"] != "foo.go" {
		t.Errorf("Attributes[file_path] = %q, want %q", event.Attributes["file_path"], "foo.go")
	}
}

func TestParseToolEvent_Ended(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"ended","name":"read","duration_ms":123}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Name != "read" {
		t.Errorf("Name = %q, want %q", event.Name, "read")
	}
	if event.Started {
		t.Error("Started = true, want false")
	}
	if event.Attributes["duration_ms"] != "123" {
		t.Errorf("Attributes[duration_ms] = %q, want %q", event.Attributes["duration_ms"], "123")
	}
}

func TestParseToolEvent_ShellCommand(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started","name":"shell","arguments":{"command":"ls -la"}}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Name != "shell" {
		t.Errorf("Name = %q, want %q", event.Name, "shell")
	}
	if event.Attributes["command"] != "ls -la" {
		t.Errorf("Attributes[command] = %q, want %q", event.Attributes["command"], "ls -la")
	}
}

func TestParseToolEvent_SearchQuery(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started","name":"search","arguments":{"query":"find function"}}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Name != "search" {
		t.Errorf("Name = %q, want %q", event.Name, "search")
	}
	if event.Attributes["query"] != "find function" {
		t.Errorf("Attributes[query] = %q, want %q", event.Attributes["query"], "find function")
	}
}

func TestParseToolEvent_WriteFile(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started","name":"write","arguments":{"path":"bar.go","file_path":"bar.go"}}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Name != "write" {
		t.Errorf("Name = %q, want %q", event.Name, "write")
	}
	if event.Attributes["file_path"] != "bar.go" {
		t.Errorf("Attributes[file_path] = %q, want %q", event.Attributes["file_path"], "bar.go")
	}
}

func TestParseToolEvent_NotToolCall(t *testing.T) {
	jsonLine := `{"type":"result","chatId":"chat-123"}`
	event := ParseToolEvent(jsonLine)
	if event != nil {
		t.Errorf("ParseToolEvent returned %+v, want nil", event)
	}
}

func TestParseToolEvent_EmptyString(t *testing.T) {
	event := ParseToolEvent("")
	if event != nil {
		t.Errorf("ParseToolEvent returned %+v, want nil", event)
	}
}

func TestParseToolEvent_InvalidJSON(t *testing.T) {
	event := ParseToolEvent("not json")
	if event != nil {
		t.Errorf("ParseToolEvent returned %+v, want nil", event)
	}
}

func TestParseToolEvent_MissingName(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started"}`
	event := ParseToolEvent(jsonLine)
	if event != nil {
		t.Errorf("ParseToolEvent returned %+v, want nil", event)
	}
}

func TestParseToolEvent_GenericAttributes(t *testing.T) {
	jsonLine := `{"type":"tool_call","subtype":"started","name":"custom_tool","arguments":{"arg1":"value1","arg2":"value2"},"extra_field":"extra_value"}`
	event := ParseToolEvent(jsonLine)
	if event == nil {
		t.Fatal("ParseToolEvent returned nil")
	}
	if event.Attributes["arg1"] != "value1" {
		t.Errorf("Attributes[arg1] = %q, want %q", event.Attributes["arg1"], "value1")
	}
	if event.Attributes["arg2"] != "value2" {
		t.Errorf("Attributes[arg2] = %q, want %q", event.Attributes["arg2"], "value2")
	}
	if event.Attributes["extra_field"] != "extra_value" {
		t.Errorf("Attributes[extra_field] = %q, want %q", event.Attributes["extra_field"], "extra_value")
	}
}
// ---------------------------------------------------------------------------
// toolEventWriter tests
// ---------------------------------------------------------------------------

type testToolObserver struct {
	starts []ToolEvent
	ends   []ToolEvent
}

func (o *testToolObserver) OnToolStart(event ToolEvent) {
	o.starts = append(o.starts, event)
}

func (o *testToolObserver) OnToolEnd(event ToolEvent) {
	o.ends = append(o.ends, event)
}

func (o *testToolObserver) OnLoopStart(string)            {}
func (o *testToolObserver) OnBeadStart(beads.Bead)         {}
func (o *testToolObserver) OnBeadComplete(BeadResult)     {}
func (o *testToolObserver) OnLoopEnd(*CoreResult)         {}

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
	if observer.starts[0].Name != "read_file" {
		t.Errorf("name = %q, want %q", observer.starts[0].Name, "read_file")
	}
	if path := observer.starts[0].Attributes["file_path"]; path != "foo.go" {
		t.Errorf("Attributes[file_path] = %q, want %q", path, "foo.go")
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
	if observer.ends[0].Name != "read_file" {
		t.Errorf("name = %q, want %q", observer.ends[0].Name, "read_file")
	}
	if durationMs := observer.ends[0].Attributes["duration_ms"]; durationMs != "150" {
		t.Errorf("Attributes[duration_ms] = %q, want %q", durationMs, "150")
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

// ---------------------------------------------------------------------------
// toolEventWriter tests
// ---------------------------------------------------------------------------

// mockObserver tracks calls to observer methods for testing.
type mockObserver struct {
	NoopObserver
	toolStarts []ToolEvent
	toolEnds   []ToolEvent
}

func (m *mockObserver) OnToolStart(event ToolEvent) {
	m.toolStarts = append(m.toolStarts, event)
}

func (m *mockObserver) OnToolEnd(event ToolEvent) {
	m.toolEnds = append(m.toolEnds, event)
}

func TestToolEventWriter_CompleteLine(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	line := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}` + "\n"
	n, err := writer.Write([]byte(line))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(line) {
		t.Errorf("Write returned %d, want %d", n, len(line))
	}

	// Inner writer should have received the data
	if inner.String() != line {
		t.Errorf("inner writer = %q, want %q", inner.String(), line)
	}

	// Observer should have been called
	if len(obs.toolStarts) != 1 {
		t.Fatalf("OnToolStart called %d times, want 1", len(obs.toolStarts))
	}
	if obs.toolStarts[0].Name != "read" {
		t.Errorf("tool name = %q, want %q", obs.toolStarts[0].Name, "read")
	}
	if !obs.toolStarts[0].Started {
		t.Error("Started = false, want true")
	}
	if len(obs.toolEnds) != 0 {
		t.Errorf("OnToolEnd called %d times, want 0", len(obs.toolEnds))
	}
}

func TestToolEventWriter_PartialLine(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	// Write partial line (no newline)
	partial := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}`
	n, err := writer.Write([]byte(partial))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(partial) {
		t.Errorf("Write returned %d, want %d", n, len(partial))
	}

	// Inner writer should have received the data
	if inner.String() != partial {
		t.Errorf("inner writer = %q, want %q", inner.String(), partial)
	}

	// Observer should NOT have been called yet (no complete line)
	if len(obs.toolStarts) != 0 {
		t.Errorf("OnToolStart called %d times, want 0 (partial line)", len(obs.toolStarts))
	}

	// Complete the line
	nl := "\n"
	n, err = writer.Write([]byte(nl))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(nl) {
		t.Errorf("Write returned %d, want %d", n, len(nl))
	}

	// Now observer should have been called
	if len(obs.toolStarts) != 1 {
		t.Fatalf("OnToolStart called %d times, want 1", len(obs.toolStarts))
	}
}

func TestToolEventWriter_MultipleLines(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	lines := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}
{"type":"tool_call","subtype":"ended","name":"read","duration_ms":123}
{"type":"result","chatId":"chat-123"}
`
	n, err := writer.Write([]byte(lines))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(lines) {
		t.Errorf("Write returned %d, want %d", n, len(lines))
	}

	// Should have two tool events
	if len(obs.toolStarts) != 1 {
		t.Errorf("OnToolStart called %d times, want 1", len(obs.toolStarts))
	}
	if len(obs.toolEnds) != 1 {
		t.Errorf("OnToolEnd called %d times, want 1", len(obs.toolEnds))
	}

	// Check start event
	if obs.toolStarts[0].Name != "read" || !obs.toolStarts[0].Started {
		t.Errorf("start event = %+v, want name=read, started=true", obs.toolStarts[0])
	}

	// Check end event
	if obs.toolEnds[0].Name != "read" || obs.toolEnds[0].Started {
		t.Errorf("end event = %+v, want name=read, started=false", obs.toolEnds[0])
	}
	if obs.toolEnds[0].Attributes["duration_ms"] != "123" {
		t.Errorf("duration_ms = %q, want %q", obs.toolEnds[0].Attributes["duration_ms"], "123")
	}
}

func TestToolEventWriter_NonToolCallIgnored(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	line := `{"type":"result","chatId":"chat-123"}` + "\n"
	n, err := writer.Write([]byte(line))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(line) {
		t.Errorf("Write returned %d, want %d", n, len(line))
	}

	// Inner writer should have received the data
	if inner.String() != line {
		t.Errorf("inner writer = %q, want %q", inner.String(), line)
	}

	// Observer should NOT have been called
	if len(obs.toolStarts) != 0 {
		t.Errorf("OnToolStart called %d times, want 0", len(obs.toolStarts))
	}
	if len(obs.toolEnds) != 0 {
		t.Errorf("OnToolEnd called %d times, want 0", len(obs.toolEnds))
	}
}

func TestToolEventWriter_InvalidJSONIgnored(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	line := "not json\n"
	n, err := writer.Write([]byte(line))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(line) {
		t.Errorf("Write returned %d, want %d", n, len(line))
	}

	// Inner writer should have received the data
	if inner.String() != line {
		t.Errorf("inner writer = %q, want %q", inner.String(), line)
	}

	// Observer should NOT have been called
	if len(obs.toolStarts) != 0 {
		t.Errorf("OnToolStart called %d times, want 0", len(obs.toolStarts))
	}
}

func TestToolEventWriter_NilObserver(t *testing.T) {
	var inner bytes.Buffer
	writer := NewToolEventWriter(&inner, nil)

	// Should return the inner writer directly (not wrapped)
	// Try to assert as toolEventWriter - should fail if it's the inner writer
	if _, ok := writer.(*toolEventWriter); ok {
		t.Error("NewToolEventWriter with nil observer should return inner writer directly, not toolEventWriter")
	}

	// Verify it's the same writer by writing to it
	testData := []byte("test")
	n, err := writer.Write(testData)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned %d, want %d", n, len(testData))
	}
	if inner.String() != "test" {
		t.Errorf("inner writer = %q, want %q", inner.String(), "test")
	}
}

func TestToolEventWriter_ChunkedWrite(t *testing.T) {
	var inner bytes.Buffer
	obs := &mockObserver{}
	writer := NewToolEventWriter(&inner, obs).(*toolEventWriter)

	// Write line in multiple chunks
	chunk1 := `{"type":"tool_call","subtype":"started","name":"read","arguments":{"path":"foo.go"}}`
	chunk2 := "\n"
	chunk3 := `{"type":"tool_call","subtype":"ended","name":"read"}` + "\n"

	writer.Write([]byte(chunk1))
	if len(obs.toolStarts) != 0 {
		t.Error("OnToolStart called before complete line")
	}

	writer.Write([]byte(chunk2))
	if len(obs.toolStarts) != 1 {
		t.Errorf("OnToolStart called %d times after newline, want 1", len(obs.toolStarts))
	}

	writer.Write([]byte(chunk3))
	if len(obs.toolEnds) != 1 {
		t.Errorf("OnToolEnd called %d times, want 1", len(obs.toolEnds))
	}
}
