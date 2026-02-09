// Package ralph implements the autonomous agent work loop.
package ralph

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	args := []string{"--model", "composer-1", "--print", "--force", "--output-format", "stream-json", prompt}
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
