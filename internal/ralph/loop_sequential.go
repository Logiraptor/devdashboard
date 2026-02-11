package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"devdeploy/internal/beads"
)

// sequentialLoopSetup holds setup state for sequential loop.
type sequentialLoopSetup struct {
	out                 io.Writer
	summary             *RunSummary
	pickNext            func() (*beads.Bead, error)
	fetchPrompt         func(beadID string) (*PromptData, error)
	render              func(data *PromptData) (string, error)
	execute             func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn            func(beadID string, result *AgentResult) (Outcome, string)
	syncFn              func() error
	currentFormatter    *LogFormatter
	consecutiveLimit    int
	consecutiveFailures int
	lastFailedBeadID    string
	skippedBeads        map[string]bool
}

// setupSequentialLoop initializes the sequential loop setup.
func setupSequentialLoop(ctx context.Context, cfg LoopConfig) (context.Context, *sequentialLoopSetup, func(), error) {
	resolved := resolveConfig(cfg)

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, resolved.wallTimeout)

	pickNext := cfg.PickNext
	if pickNext == nil {
		if cfg.TargetBead != "" {
			// When TargetBead is set, create a picker that always returns that bead
			// (only on first call, then returns nil)
			once := false
			pickNext = func() (*beads.Bead, error) {
				if once {
					return nil, nil // Signal no more beads
				}
				once = true
				// Use bd list to get full bead info (includes all fields we need)
				cmd := exec.Command("bd", "list", "--json", "--limit", "1")
				cmd.Dir = cfg.WorkDir
				// Filter by ID using grep-like approach, or use bd show and construct minimal Bead
				// Actually, let's use bd list with a filter, but bd list doesn't filter by ID directly
				// So we'll use bd show to verify it exists and get title, then construct Bead
				showCmd := exec.Command("bd", "show", cfg.TargetBead, "--json")
				showCmd.Dir = cfg.WorkDir
				outBytes, err := showCmd.Output()
				if err != nil {
					return nil, fmt.Errorf("bd show %s: %w", cfg.TargetBead, err)
				}
				// bd show returns an array with one entry containing id, title, description
				var showEntries []struct {
					ID    string `json:"id"`
					Title string `json:"title"`
				}
				if err := json.Unmarshal(outBytes, &showEntries); err != nil {
					return nil, fmt.Errorf("parsing bd show output: %w", err)
				}
				if len(showEntries) == 0 {
					return nil, fmt.Errorf("bead %s not found", cfg.TargetBead)
				}
				e := showEntries[0]
				// Construct a minimal Bead - other fields will have defaults
				// Status, Priority, Labels, CreatedAt will be empty/zero, which is fine
				// for our purposes since we just need ID and Title for the prompt
				return &beads.Bead{
					ID:    e.ID,
					Title: e.Title,
					// Default values for other fields are fine
				}, nil
			}
		} else {
			picker := &BeadPicker{
				WorkDir: cfg.WorkDir,
				Epic:    cfg.Epic,
			}
			pickNext = picker.Next
		}
	}

	// Track formatter for summary output
	var currentFormatter *LogFormatter

	execute := cfg.Execute
	if execute == nil {
		execute = createExecuteFn(cfg, resolved.out, &currentFormatter)
	}

	summary := &RunSummary{}
	consecutiveFailures := 0
	lastFailedBeadID := ""
	skippedBeads := make(map[string]bool)

	setup := &sequentialLoopSetup{
		out:                 resolved.out,
		summary:             summary,
		pickNext:            pickNext,
		fetchPrompt:         resolved.fetchPrompt,
		render:              resolved.render,
		execute:             execute,
		assessFn:            resolved.assessFn,
		syncFn:              resolved.syncFn,
		currentFormatter:    currentFormatter,
		consecutiveLimit:    resolved.consecutiveLimit,
		consecutiveFailures:  consecutiveFailures,
		lastFailedBeadID:    lastFailedBeadID,
		skippedBeads:        skippedBeads,
	}

	cleanup := func() {
		cancelWall()
	}

	return ctx, setup, cleanup, nil
}

// handleSameBeadRetry handles same-bead retry detection logic.
func handleSameBeadRetry(setup *sequentialLoopSetup, bead *beads.Bead, cfg LoopConfig, loopStart time.Time, i int) (*beads.Bead, bool, error) {
	if setup.lastFailedBeadID == "" || bead.ID != setup.lastFailedBeadID {
		return bead, true, nil
	}

	setup.skippedBeads[bead.ID] = true
	setup.summary.Skipped++
	setup.lastFailedBeadID = "" // reset so we don't skip indefinitely

	// Check if we should stop: if we've skipped beads and the picker
	// keeps returning skipped beads, we're in an infinite loop.
	// Try one more pick; if that's also skipped, stop.
	retryBead, retryErr := setup.pickNext()
	if retryErr != nil {
		setup.summary.Duration = time.Since(loopStart)
		return nil, false, fmt.Errorf("iteration %d: picking bead (retry): %w", i+1, retryErr)
	}
	if retryBead == nil {
		setup.summary.StopReason = StopNormal
		return nil, false, nil
	}
	if setup.skippedBeads[retryBead.ID] {
		setup.summary.Skipped++
		setup.summary.StopReason = StopAllBeadsSkipped
		return nil, false, nil
	}
	return retryBead, true, nil
}

// executeAgentForBead fetches prompt, renders it, executes agent, and prints summary.
func executeAgentForBead(ctx context.Context, cfg LoopConfig, setup *sequentialLoopSetup, bead *beads.Bead, loopStart time.Time, i int) (string, *AgentResult, error) {
	// Fetch full bead data and render prompt.
	promptData, err := setup.fetchPrompt(bead.ID)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: fetching prompt data for %s: %w",
			i+1, bead.ID, err)
	}

	prompt, err := setup.render(promptData)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: rendering prompt for %s: %w",
			i+1, bead.ID, err)
	}

	// Get commit hash before agent execution for landing check.
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = cfg.WorkDir
	commitHashBefore := ""
	if outBytes, err := cmd.Output(); err == nil {
		commitHashBefore = strings.TrimSpace(string(outBytes))
	}

	// Set current bead for progress display
	if setup.currentFormatter != nil {
		setup.currentFormatter.SetCurrentBead(bead.ID, bead.Title)
	}

	// Execute agent.
	result, err := setup.execute(ctx, prompt)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return "", nil, fmt.Errorf("iteration %d: running agent for %s: %w",
			i+1, bead.ID, err)
	}

	// Print summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		// Clear progress line before printing summary
		writef(setup.out, "\n")
		if summaryStr := setup.currentFormatter.Summary(); summaryStr != "" {
			writef(setup.out, "%s\n", summaryStr)
		}
		setup.currentFormatter = nil // Reset for next iteration
	}

	return commitHashBefore, result, nil
}

// handleLandingAndLogging checks landing status and logs iteration results.
func handleLandingAndLogging(cfg LoopConfig, setup *sequentialLoopSetup, bead *beads.Bead, commitHashBefore string, outcome Outcome, outcomeSummary string, result *AgentResult, i int) Outcome {
	// Check landing status.
	landingStatus, landingErr := CheckLanding(cfg.WorkDir, bead.ID, commitHashBefore)
	if landingErr == nil {
		landingMsg := FormatLandingStatus(landingStatus)
		if landingMsg != "landed successfully" {
			writef(setup.out, "  Landing: %s\n", landingMsg)
			// If strict landing is enabled and landing is incomplete, treat as failure
			if cfg.StrictLanding && outcome == OutcomeSuccess {
				// Override success if landing is incomplete
				if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
					outcome = OutcomeFailure
					outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
				}
			}
		} else {
			writef(setup.out, "  Landing: %s\n", landingMsg)
		}
	}

	// Print structured per-iteration log line.
	writef(setup.out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, outcome, result.Duration, outcomeSummary))
	// Print bead summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		if summary := setup.currentFormatter.BeadSummary(); summary != "" {
			writef(setup.out, "%s\n", summary)
		}
	}

	return outcome
}

// printVerboseOutput prints verbose agent output (stdout/stderr excerpts).
func printVerboseOutput(out io.Writer, result *AgentResult) {
	if result.Stdout != "" {
		lines := strings.Split(result.Stdout, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			writef(out, "  stdout (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				writef(out, "    %s\n", line)
			}
		} else {
			writef(out, "  stdout:\n")
			for _, line := range lines {
				if line != "" {
					writef(out, "    %s\n", line)
				}
			}
		}
	}
	if result.Stderr != "" {
		lines := strings.Split(result.Stderr, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			writef(out, "  stderr (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				writef(out, "    %s\n", line)
			}
		} else {
			writef(out, "  stderr:\n")
			for _, line := range lines {
				if line != "" {
					writef(out, "    %s\n", line)
				}
			}
		}
	}
}

// updateOutcomeCounters updates counters based on outcome.
func updateOutcomeCounters(setup *sequentialLoopSetup, bead *beads.Bead, outcome Outcome) {
	setup.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		setup.summary.Succeeded++
		setup.consecutiveFailures = 0
		setup.lastFailedBeadID = ""
	case OutcomeQuestion:
		setup.summary.Questions++
		setup.consecutiveFailures = 0
		setup.lastFailedBeadID = ""
	case OutcomeFailure:
		setup.summary.Failed++
		setup.consecutiveFailures++
		setup.lastFailedBeadID = bead.ID
	case OutcomeTimeout:
		setup.summary.TimedOut++
		setup.consecutiveFailures++
		setup.lastFailedBeadID = bead.ID
	}
}

// processSequentialIteration processes a single sequential iteration.
func processSequentialIteration(ctx context.Context, cfg LoopConfig, setup *sequentialLoopSetup, loopStart time.Time, i int) (bool, error) {
	// Guard: context cancellation / wall-clock timeout.
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			setup.summary.StopReason = StopWallClock
		} else {
			setup.summary.StopReason = StopContextCancelled
		}
		return false, nil
	}

	// 1. Pick next bead.
	bead, err := setup.pickNext()
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: picking bead: %w", i+1, err)
	}
	if bead == nil {
		setup.summary.StopReason = StopNormal
		return false, nil
	}

	// Guard: same-bead retry detection.
	bead, shouldContinue, err := handleSameBeadRetry(setup, bead, cfg, loopStart, i)
	if err != nil {
		return false, err
	}
	if !shouldContinue {
		return false, nil
	}

	// Dry-run: print what would be done without executing.
	if cfg.DryRun {
		writef(setup.out, "%s\n", formatIterationLog(i+1, cfg.MaxIterations, bead.ID, bead.Title, OutcomeSuccess, 0, ""))
		setup.summary.Iterations++
		return false, nil
	}

	// 2. Execute agent for bead.
	commitHashBefore, result, err := executeAgentForBead(ctx, cfg, setup, bead, loopStart, i)
	if err != nil {
		return false, err
	}

	// 3. Assess outcome.
	outcome, outcomeSummary := setup.assessFn(bead.ID, result)

	// 4. Handle landing and logging.
	outcome = handleLandingAndLogging(cfg, setup, bead, commitHashBefore, outcome, outcomeSummary, result, i)

	// Verbose mode: print agent stdout/stderr excerpts.
	if cfg.Verbose {
		printVerboseOutput(setup.out, result)
	}

	// 5. Update counters.
	updateOutcomeCounters(setup, bead, outcome)

	// Guard: consecutive failure limit.
	if setup.consecutiveFailures >= setup.consecutiveLimit {
		setup.summary.StopReason = StopConsecutiveFails
		return false, nil
	}

	// 6. Sync beads state.
	if err := setup.syncFn(); err != nil {
		if cfg.Verbose {
			writef(setup.out, "  bd sync warning: %v\n", err)
		}
	}

	return true, nil
}

// writeFinalSequentialStatus writes the final summary for sequential loop.
func writeFinalSequentialStatus(cfg LoopConfig, setup *sequentialLoopSetup, loopStart time.Time) {
	// If we exhausted all iterations without an earlier stop reason, set it.
	if setup.summary.StopReason == StopNormal && setup.summary.Iterations >= cfg.MaxIterations {
		setup.summary.StopReason = StopMaxIterations
	}

	// Calculate total duration.
	setup.summary.Duration = time.Since(loopStart)

	// Count remaining beads.
	remainingBeads := countRemainingBeads(cfg)

	// Print final summary (always printed, even on early termination).
	writef(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))
}

// runSequential executes the sequential loop (original implementation).
func runSequential(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	// Epic mode: when Epic is set and TargetBead is empty, orchestrate children sequentially
	if cfg.Epic != "" && cfg.TargetBead == "" {
		return runEpicOrchestrator(ctx, cfg)
	}

	ctx, setup, cleanup, err := setupSequentialLoop(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loopStart := time.Now()

	for i := 0; i < cfg.MaxIterations; i++ {
		shouldContinue, err := processSequentialIteration(ctx, cfg, setup, loopStart, i)
		if err != nil {
			return setup.summary, err
		}
		if !shouldContinue {
			break
		}
	}

	// Write final status and summary
	writeFinalSequentialStatus(cfg, setup, loopStart)

	return setup.summary, nil
}
