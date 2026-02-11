package ralph

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestTraceWriter_PassesThroughData(t *testing.T) {
	// Create a real trace client (it will disable itself if server unavailable)
	traceClient := NewTraceClient()
	var inner bytes.Buffer
	writer := NewTraceWriter(&inner, traceClient)

	// Write a tool_call event (nested schema)
	startEvent := map[string]interface{}{
		"type":    "tool_call",
		"subtype": "started",
		"id":      "tool-1",
		"tool_call": map[string]interface{}{
			"readToolCall": map[string]interface{}{
				"args": map[string]interface{}{
					"target_file": "/path/to/file.go",
				},
			},
		},
	}
	startJSON, _ := json.Marshal(startEvent)
	_, err := writer.Write(append(startJSON, '\n'))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify inner writer received the data
	innerStr := inner.String()
	if !bytes.Contains([]byte(innerStr), []byte(`"type":"tool_call"`)) {
		t.Error("inner writer should have received tool_call event")
	}
}

func TestTraceWriter_HandlesPartialLines(t *testing.T) {
	traceClient := NewTraceClient()
	var inner bytes.Buffer
	writer := NewTraceWriter(&inner, traceClient)

	// Write partial line
	partial := []byte(`{"type":"tool_call","subtype":"started","id":"tool-3","tool_call":{"editToolCall":{"args":{"path":"test.go"}}`)
	n1, err := writer.Write(partial)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n1 != len(partial) {
		t.Errorf("expected Write to return %d, got %d", len(partial), n1)
	}

	// Complete the line
	completion := []byte(`}}}\n`)
	n2, err := writer.Write(completion)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n2 != len(completion) {
		t.Errorf("expected Write to return %d, got %d", len(completion), n2)
	}

	// Verify inner writer received complete data
	innerStr := inner.String()
	if !bytes.Contains([]byte(innerStr), []byte(`"path":"test.go"`)) {
		t.Error("inner writer should have received complete event after partial write")
	}
}

func TestTraceWriter_IgnoresNonToolCallEvents(t *testing.T) {
	traceClient := NewTraceClient()
	var inner bytes.Buffer
	writer := NewTraceWriter(&inner, traceClient)

	// Write a non-tool_call event
	event := map[string]interface{}{
		"type": "assistant",
		"content": "Hello",
	}
	eventJSON, _ := json.Marshal(event)
	_, err := writer.Write(append(eventJSON, '\n'))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify inner writer still received the data
	innerStr := inner.String()
	if !bytes.Contains([]byte(innerStr), []byte(`"type":"assistant"`)) {
		t.Error("inner writer should have received non-tool_call event")
	}
}
