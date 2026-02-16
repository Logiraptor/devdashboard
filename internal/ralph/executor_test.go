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
