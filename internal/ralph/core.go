// Package ralph implements the autonomous agent work loop.
// core.go provides a single-bead iteration loop.
package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// DefaultMaxIterations is the default number of agent iterations per bead.
const DefaultMaxIterations = 10

// ToolEvent represents a tool call event.
type ToolEvent struct {
	ID         string            // Unique identifier for this tool call
	Name       string            // Tool name
	Started    bool              // True for start events, false for end events
	Timestamp  time.Time         // When the event occurred
	Attributes map[string]string // Tool attributes
	BeadID     string            // ID of the bead this event belongs to
}

// ProgressObserver receives progress updates from Core execution.
// All methods are optional — implement only what you need.
// Methods are called synchronously from the execution goroutine.
type ProgressObserver interface {
	// OnLoopStart is called when the loop begins.
	OnLoopStart(rootBead string)

	// OnLoopEnd is called when the loop completes.
	OnLoopEnd(result *CoreResult)

	// OnBeadStart is called when work begins on a bead.
	OnBeadStart(bead beads.Bead)

	// OnBeadComplete is called when a bead finishes (success or failure).
	OnBeadComplete(result BeadResult)

	// OnToolStart is called when a tool call starts.
	OnToolStart(event ToolEvent)

	// OnToolEnd is called when a tool call ends.
	OnToolEnd(event ToolEvent)

	// OnIterationStart is called when an iteration begins.
	OnIterationStart(iteration int)

	// OnVerifyStart is called when verification phase begins.
	OnVerifyStart(beadID string)

	// OnVerifyEnd is called when verification phase ends.
	OnVerifyEnd(result VerifyResult)
}

// VerifyResult holds the result of a verification pass.
type VerifyResult struct {
	BeadID        string
	NewBeads      []string      // IDs of new beads created by verifier
	BeadReopened  bool          // True if verifier reopened the bead
	Duration      time.Duration
	AgentResult   *AgentResult
}

// NoopObserver is a ProgressObserver that does nothing.
// Embed this in your observer to avoid implementing unused methods.
type NoopObserver struct{}

// Ensure NoopObserver implements ProgressObserver.
var _ ProgressObserver = (*NoopObserver)(nil)

func (NoopObserver) OnLoopStart(string)         {}
func (NoopObserver) OnBeadStart(beads.Bead)     {}
func (NoopObserver) OnBeadComplete(BeadResult)  {}
func (NoopObserver) OnLoopEnd(*CoreResult)      {}
func (NoopObserver) OnToolStart(ToolEvent)      {}
func (NoopObserver) OnToolEnd(ToolEvent)        {}
func (NoopObserver) OnIterationStart(int)       {}
func (NoopObserver) OnVerifyStart(string)       {}
func (NoopObserver) OnVerifyEnd(VerifyResult)   {}

// Core orchestrates agent execution for a single bead.
type Core struct {
	// WorkDir is the root repository directory.
	WorkDir string

	// RootBead is the bead ID to work on (required).
	RootBead string

	// MaxIterations is the maximum agent iterations before giving up.
	// Zero means use DefaultMaxIterations (10).
	MaxIterations int

	// AgentTimeout is the per-agent execution timeout.
	// Zero means use DefaultTimeout (10m).
	AgentTimeout time.Duration

	// EnableVerify enables verification pass after bead closure.
	// When true, an opus 4.5 thinking agent reviews the work after the
	// composer-1 agent closes the bead. If the verifier creates new beads,
	// the loop continues with composer-1.
	EnableVerify bool

	// Output is where logs are written. Defaults to os.Stdout.
	Output io.Writer

	// Observer receives progress updates. Optional.
	Observer ProgressObserver

	// Test hooks (nil means use real implementations)
	RunBD         bd.Runner
	FetchPrompt   func(runBD bd.Runner, workDir, beadID string) (*PromptData, error)
	Render        func(data *PromptData) (string, error)
	RenderVerify  func(data *PromptData) (string, error)
	Execute       func(ctx context.Context, workDir, prompt string) (*AgentResult, error)
	ExecuteVerify func(ctx context.Context, workDir, prompt string) (*AgentResult, error)
	AssessFn      func(workDir, beadID string, result *AgentResult) (Outcome, string)
}

// CoreResult holds the results of a Core.Run invocation.
type CoreResult struct {
	Outcome          Outcome
	Iterations       int
	VerifyIterations int // Number of verification passes
	Duration         time.Duration
}

// Run executes the ralph loop on a single bead.
// Iterates until the bead is closed, max iterations reached, or a stopping condition.
// If EnableVerify is true, runs an opus verification pass after bead closure.
// If the verifier creates new beads that block the target, continues looping.
func (c *Core) Run(ctx context.Context) (*CoreResult, error) {
	start := time.Now()
	result := &CoreResult{}

	out := c.Output
	if out == nil {
		out = os.Stdout
	}

	maxIter := c.MaxIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxIterations
	}

	// Notify observer of loop start
	if c.Observer != nil {
		c.Observer.OnLoopStart(c.RootBead)
	}

	// Fetch initial bead info
	bead, err := c.fetchBead()
	if err != nil {
		return nil, fmt.Errorf("fetching bead %s: %w", c.RootBead, err)
	}

	// Notify observer of bead start
	if c.Observer != nil {
		c.Observer.OnBeadStart(bead)
	}

	var lastAgentResult *AgentResult
	var lastOutcome Outcome
	var lastSummary string
	iteration := 0

	// Main loop: continues if verification reopens the bead
	for iteration < maxIter {
		// Check context cancellation
		if ctx.Err() != nil {
			result.Outcome = OutcomeTimeout
			result.Iterations = iteration
			result.Duration = time.Since(start)
			c.notifyComplete(bead, result, lastAgentResult, "context cancelled")
			return result, nil
		}

		// Notify observer of iteration start
		if c.Observer != nil {
			c.Observer.OnIterationStart(iteration)
		}

		// Check if bead is already closed
		closed, err := c.isBeadClosed()
		if err != nil {
			writef(out, "[%s] error checking bead status: %v\n", c.RootBead, err)
		}
		if closed {
			// Bead is closed - check if we need verification
			if c.EnableVerify {
				writef(out, "[%s] bead closed, running verification...\n", c.RootBead)
				reopened, verifyErr := c.runVerification(ctx, out)
				result.VerifyIterations++
				if verifyErr != nil {
					writef(out, "[%s] verification error: %v\n", c.RootBead, verifyErr)
				}
				if reopened {
					writef(out, "[%s] verification reopened bead, continuing...\n", c.RootBead)
					// Refresh bead info after verification
					bead, _ = c.fetchBead()
					continue // Continue loop
				}
			}

			result.Outcome = OutcomeSuccess
			result.Iterations = iteration
			result.Duration = time.Since(start)
			writef(out, "[%s] bead closed after %d iteration(s)\n", c.RootBead, iteration)
			c.notifyComplete(bead, result, lastAgentResult, "")
			if c.Observer != nil {
				c.Observer.OnLoopEnd(result)
			}
			return result, nil
		}

		writef(out, "[%s] iteration %d/%d\n", c.RootBead, iteration+1, maxIter)

		// Fetch and render prompt
		prompt, err := c.buildPrompt()
		if err != nil {
			writef(out, "[%s] failed to build prompt: %v\n", c.RootBead, err)
			result.Outcome = OutcomeFailure
			result.Iterations = iteration + 1
			result.Duration = time.Since(start)
			c.notifyComplete(bead, result, nil, err.Error())
			return result, nil
		}

		// Execute agent
		agentResult, err := c.runAgent(ctx, prompt)
		if err != nil {
			writef(out, "[%s] agent execution error: %v\n", c.RootBead, err)
			result.Outcome = OutcomeFailure
			result.Iterations = iteration + 1
			result.Duration = time.Since(start)
			c.notifyComplete(bead, result, agentResult, err.Error())
			return result, nil
		}
		lastAgentResult = agentResult

		// Assess outcome
		outcome, summary := c.assessOutcome(agentResult)
		lastOutcome = outcome
		lastSummary = summary

		iteration++ // Increment after running agent

		writef(out, "[%s] iteration %d → %s (%s)\n", c.RootBead, iteration, outcome, FormatDuration(agentResult.Duration))
		if summary != "" && outcome != OutcomeSuccess {
			writef(out, "  %s\n", summary)
		}

		// Stop on question or timeout (non-retryable)
		if outcome == OutcomeQuestion || outcome == OutcomeTimeout {
			result.Outcome = outcome
			result.Iterations = iteration
			result.Duration = time.Since(start)
			c.notifyComplete(bead, result, agentResult, summary)
			if c.Observer != nil {
				c.Observer.OnLoopEnd(result)
			}
			return result, nil
		}

		// On success, check if bead is closed (will happen next iteration)
		// On failure, continue to retry
	}

	// Max iterations reached
	result.Outcome = OutcomeMaxIterations
	result.Iterations = iteration
	result.Duration = time.Since(start)

	// Use the last outcome if we have one
	if lastOutcome != 0 {
		writef(out, "[%s] max iterations reached (last outcome: %s)\n", c.RootBead, lastOutcome)
	} else {
		writef(out, "[%s] max iterations reached\n", c.RootBead)
	}

	c.notifyComplete(bead, result, lastAgentResult, lastSummary)

	// Notify observer of loop end
	if c.Observer != nil {
		c.Observer.OnLoopEnd(result)
	}

	return result, nil
}

// runVerification executes the opus verification pass.
// Returns true if the bead was reopened (new blocking beads were created).
func (c *Core) runVerification(ctx context.Context, out io.Writer) (bool, error) {
	// Notify observer
	if c.Observer != nil {
		c.Observer.OnVerifyStart(c.RootBead)
	}

	verifyStart := time.Now()

	// Build verification prompt
	promptData, err := c.fetchPromptData()
	if err != nil {
		return false, fmt.Errorf("fetching prompt data: %w", err)
	}

	renderVerify := c.RenderVerify
	if renderVerify == nil {
		renderVerify = RenderVerifyPrompt
	}
	prompt, err := renderVerify(promptData)
	if err != nil {
		return false, fmt.Errorf("rendering verify prompt: %w", err)
	}

	// Execute verification agent (opus)
	var agentResult *AgentResult
	if c.ExecuteVerify != nil {
		agentResult, err = c.ExecuteVerify(ctx, c.WorkDir, prompt)
	} else {
		var opts []Option
		if c.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(c.AgentTimeout))
		}
		if c.Observer != nil {
			opts = append(opts, WithObserver(c.Observer))
			opts = append(opts, WithStdoutWriter(io.Discard))
		}
		agentResult, err = RunAgentOpus(ctx, c.WorkDir, prompt, opts...)
	}

	verifyResult := VerifyResult{
		BeadID:      c.RootBead,
		Duration:    time.Since(verifyStart),
		AgentResult: agentResult,
	}

	if err != nil {
		if c.Observer != nil {
			c.Observer.OnVerifyEnd(verifyResult)
		}
		return false, fmt.Errorf("verification agent: %w", err)
	}

	writef(out, "[%s] verification completed (%s)\n", c.RootBead, FormatDuration(agentResult.Duration))

	// Check if bead was reopened
	closed, err := c.isBeadClosed()
	if err != nil {
		if c.Observer != nil {
			c.Observer.OnVerifyEnd(verifyResult)
		}
		return false, fmt.Errorf("checking bead status: %w", err)
	}

	verifyResult.BeadReopened = !closed

	// Notify observer
	if c.Observer != nil {
		c.Observer.OnVerifyEnd(verifyResult)
	}

	return !closed, nil
}

// fetchPromptData gets the prompt data for the current bead.
func (c *Core) fetchPromptData() (*PromptData, error) {
	fetchPrompt := c.FetchPrompt
	if fetchPrompt == nil {
		fetchPrompt = FetchPromptData
	}
	return fetchPrompt(c.RunBD, c.WorkDir, c.RootBead)
}

// notifyComplete sends the bead complete notification to the observer.
func (c *Core) notifyComplete(bead beads.Bead, result *CoreResult, agentResult *AgentResult, errMsg string) {
	if c.Observer == nil {
		return
	}

	br := BeadResult{
		Bead:     bead,
		Outcome:  result.Outcome,
		Duration: result.Duration,
	}
	if agentResult != nil {
		br.ChatID = agentResult.ChatID
		br.ExitCode = agentResult.ExitCode
		br.Stderr = agentResult.Stderr
	}
	if errMsg != "" {
		br.ErrorMessage = errMsg
	}
	c.Observer.OnBeadComplete(br)
}

// fetchBead retrieves the bead information from bd.
func (c *Core) fetchBead() (beads.Bead, error) {
	runner := c.RunBD
	if runner == nil {
		runner = bd.Run
	}

	out, err := runner(c.WorkDir, "show", c.RootBead, "--json")
	if err != nil {
		return beads.Bead{}, fmt.Errorf("bd show %s: %w", c.RootBead, err)
	}

	var entries []bdShowReadyEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return beads.Bead{}, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(entries) == 0 {
		return beads.Bead{}, fmt.Errorf("bead %s not found", c.RootBead)
	}

	e := entries[0]
	return beads.Bead{
		ID:        e.ID,
		Title:     e.Title,
		Status:    e.Status,
		Priority:  e.Priority,
		Labels:    e.Labels,
		CreatedAt: e.CreatedAt,
	}, nil
}

// isBeadClosed checks if the bead has been closed.
func (c *Core) isBeadClosed() (bool, error) {
	runner := c.RunBD
	if runner == nil {
		runner = bd.Run
	}

	out, err := runner(c.WorkDir, "show", c.RootBead, "--json")
	if err != nil {
		return false, fmt.Errorf("bd show %s: %w", c.RootBead, err)
	}

	var entries []bdShowReadyEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return false, fmt.Errorf("parsing bd show output: %w", err)
	}
	if len(entries) == 0 {
		return false, fmt.Errorf("bead %s not found", c.RootBead)
	}

	return entries[0].Status == "closed", nil
}

// buildPrompt fetches bead data and renders the prompt.
func (c *Core) buildPrompt() (string, error) {
	fetchPrompt := c.FetchPrompt
	if fetchPrompt == nil {
		fetchPrompt = FetchPromptData
	}

	promptData, err := fetchPrompt(c.RunBD, c.WorkDir, c.RootBead)
	if err != nil {
		return "", fmt.Errorf("fetching prompt data: %w", err)
	}

	render := c.Render
	if render == nil {
		render = RenderPrompt
	}

	return render(promptData)
}

// runAgent executes the agent with the given prompt.
func (c *Core) runAgent(ctx context.Context, prompt string) (*AgentResult, error) {
	if c.Execute != nil {
		return c.Execute(ctx, c.WorkDir, prompt)
	}

	var opts []Option
	if c.AgentTimeout > 0 {
		opts = append(opts, WithTimeout(c.AgentTimeout))
	}
	if c.Observer != nil {
		opts = append(opts, WithObserver(c.Observer))
		opts = append(opts, WithStdoutWriter(io.Discard))
	}

	return RunAgent(ctx, c.WorkDir, prompt, opts...)
}

// assessOutcome evaluates the agent result and returns the outcome.
func (c *Core) assessOutcome(result *AgentResult) (Outcome, string) {
	assessFn := c.AssessFn
	if assessFn == nil {
		assessFn = func(wd, id string, r *AgentResult) (Outcome, string) {
			return Assess(wd, id, r, nil)
		}
	}
	return assessFn(c.WorkDir, c.RootBead, result)
}
