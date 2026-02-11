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
