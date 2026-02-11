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

	return &AgentResult{
		ExitCode: exitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
		TimedOut: timedOut,
	}, nil
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
