package ralph

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"devdeploy/internal/beads"
)

// epicOrchestratorSetup holds setup state for epic orchestrator.
type epicOrchestratorSetup struct {
	out              io.Writer
	summary          *RunSummary
	epicPromptData   *PromptData
	fetchPrompt      func(beadID string) (*PromptData, error)
	render           func(data *PromptData) (string, error)
	execute          func(ctx context.Context, prompt string) (*AgentResult, error)
	assessFn         func(beadID string, result *AgentResult) (Outcome, string)
	syncFn           func() error
	currentFormatter *LogFormatter
	processedBeads   map[string]bool
	fetchChildren    func() ([]beads.Bead, error)
}

// setupEpicOrchestrator initializes the epic orchestrator setup.
func setupEpicOrchestrator(ctx context.Context, cfg LoopConfig) (context.Context, *epicOrchestratorSetup, func(), error) {
	resolved := resolveConfig(cfg)

	// Apply wall-clock timeout to context.
	ctx, cancelWall := context.WithTimeout(ctx, resolved.wallTimeout)

	summary := &RunSummary{}

	// Fetch epic info for verification prompt
	epicPromptData, err := FetchPromptData(nil, cfg.WorkDir, cfg.Epic)
	if err != nil {
		cancelWall()
		return nil, nil, nil, fmt.Errorf("fetching epic prompt data: %w", err)
	}

	// Track formatter for summary output
	var currentFormatter *LogFormatter

	execute := cfg.Execute
	if execute == nil {
		execute = createExecuteFn(cfg, resolved.out, &currentFormatter)
	}

	// Track processed beads to avoid infinite loops if a bead keeps appearing
	processedBeads := make(map[string]bool)

	// Fetch children function - can be overridden for testing
	fetchChildren := func() ([]beads.Bead, error) {
		return FetchEpicChildren(nil, cfg.WorkDir, cfg.Epic)
	}

	setup := &epicOrchestratorSetup{
		out:              resolved.out,
		summary:          summary,
		epicPromptData:   epicPromptData,
		fetchPrompt:      resolved.fetchPrompt,
		render:           resolved.render,
		execute:          execute,
		assessFn:         resolved.assessFn,
		syncFn:           resolved.syncFn,
		currentFormatter: currentFormatter,
		processedBeads:   processedBeads,
		fetchChildren:    fetchChildren,
	}

	cleanup := func() {
		cancelWall()
	}

	return ctx, setup, cleanup, nil
}

// processEpicIteration processes a single epic iteration.
func processEpicIteration(ctx context.Context, cfg LoopConfig, setup *epicOrchestratorSetup, loopStart time.Time, iteration int) (bool, error) {
	// Guard: context cancellation / wall-clock timeout.
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			setup.summary.StopReason = StopWallClock
		} else {
			setup.summary.StopReason = StopContextCancelled
		}
		return false, nil
	}

	// Query for ready leaf tasks
	if iteration == 0 {
		_, _ = fmt.Fprintf(setup.out, "Epic orchestrator: querying 'bd ready --parent %s' for leaf tasks\n", cfg.Epic)
	}
	children, err := setup.fetchChildren()
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("fetching epic children: %w", err)
	}

	// Filter out already processed beads
	readyChildren := make([]beads.Bead, 0, len(children))
	for _, child := range children {
		if !setup.processedBeads[child.ID] {
			readyChildren = append(readyChildren, child)
		}
	}

	if len(readyChildren) == 0 {
		if iteration == 0 {
			_, _ = fmt.Fprintf(setup.out, "No ready children found for epic %s\n", cfg.Epic)
		} else {
			_, _ = fmt.Fprintf(setup.out, "No more ready children for epic %s\n", cfg.Epic)
		}
		setup.summary.StopReason = StopNormal
		return false, nil
	}

	if iteration == 0 {
		_, _ = fmt.Fprintf(setup.out, "Found %d ready leaf task(s) for epic %s\n", len(readyChildren), cfg.Epic)
	}

	// Process the first ready leaf (sorted by priority)
	child := readyChildren[0]
	iterNum := iteration + 1

	// Mark as processed to avoid reprocessing
	setup.processedBeads[child.ID] = true

	// Fetch prompt data and render prompt
	promptData, err := setup.fetchPrompt(child.ID)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: fetching prompt data for %s: %w", iterNum, child.ID, err)
	}

	prompt, err := setup.render(promptData)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: rendering prompt for %s: %w", iterNum, child.ID, err)
	}

	// Get commit hash before agent execution for landing check
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = cfg.WorkDir
	commitHashBefore := ""
	if outBytes, err := cmd.Output(); err == nil {
		commitHashBefore = strings.TrimSpace(string(outBytes))
	}

	// Set current bead for progress display
	if setup.currentFormatter != nil {
		setup.currentFormatter.SetCurrentBead(child.ID, child.Title)
	}

	// Execute agent
	result, err := setup.execute(ctx, prompt)
	if err != nil {
		setup.summary.Duration = time.Since(loopStart)
		return false, fmt.Errorf("iteration %d: running agent for %s: %w", iterNum, child.ID, err)
	}

	// Print summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		// Clear progress line before printing summary
		_, _ = fmt.Fprintf(setup.out, "\n")
		if summaryStr := setup.currentFormatter.Summary(); summaryStr != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summaryStr)
		}
		setup.currentFormatter = nil // Reset for next iteration
	}

	// Assess outcome
	outcome, outcomeSummary := setup.assessFn(child.ID, result)

	// Check landing status
	landingStatus, landingErr := CheckLanding(cfg.WorkDir, child.ID, commitHashBefore)
	if landingErr == nil {
		landingMsg := FormatLandingStatus(landingStatus)
		if landingMsg != "landed successfully" {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
			if cfg.StrictLanding && outcome == OutcomeSuccess {
				if landingStatus.HasUncommittedChanges || !landingStatus.BeadClosed {
					outcome = OutcomeFailure
					outcomeSummary = fmt.Sprintf("%s; %s", outcomeSummary, landingMsg)
				}
			}
		} else {
			_, _ = fmt.Fprintf(setup.out, "  Landing: %s\n", landingMsg)
		}
	}

	// Print structured per-iteration log line
	_, _ = fmt.Fprintf(setup.out, "%s\n", formatIterationLog(iterNum, 0, child.ID, child.Title, outcome, result.Duration, outcomeSummary))
	// Print bead summary if formatter was used
	if setup.currentFormatter != nil && !cfg.Verbose {
		if summary := setup.currentFormatter.BeadSummary(); summary != "" {
			_, _ = fmt.Fprintf(setup.out, "%s\n", summary)
		}
	}

	// Update counters
	setup.summary.Iterations++
	switch outcome {
	case OutcomeSuccess:
		setup.summary.Succeeded++
	case OutcomeQuestion:
		setup.summary.Questions++
	case OutcomeFailure:
		setup.summary.Failed++
		// If a child fails, stop the epic orchestration
		setup.summary.StopReason = StopConsecutiveFails
		setup.summary.Duration = time.Since(loopStart)
		return false, nil
	case OutcomeTimeout:
		setup.summary.TimedOut++
		// If a child times out, stop the epic orchestration
		setup.summary.StopReason = StopWallClock
		setup.summary.Duration = time.Since(loopStart)
		return false, nil
	}

	// Sync beads state after each completion
	if err := setup.syncFn(); err != nil {
		if cfg.Verbose {
			_, _ = fmt.Fprintf(setup.out, "  bd sync warning: %v\n", err)
		}
	}

	return true, nil
}

// runOpusVerification runs opus verification after all epic children complete.
func runOpusVerification(ctx context.Context, cfg LoopConfig, setup *epicOrchestratorSetup) {
	if setup.summary.Failed == 0 && setup.summary.TimedOut == 0 && setup.summary.Iterations > 0 {
		_, _ = fmt.Fprintf(setup.out, "\nAll %d leaf task(s) completed successfully. Running opus verification...\n", setup.summary.Iterations)

		// Fetch all epic children (including closed) for review
		allChildren, err := FetchAllEpicChildren(nil, cfg.WorkDir, cfg.Epic)
		if err != nil {
			_, _ = fmt.Fprintf(setup.out, "Warning: failed to fetch all epic children for verification: %v\n", err)
			allChildren = nil
		}

		// Get git log to see code changes (last 50 commits, or since epic creation if we can determine it)
		var gitLog string
		cmd := exec.Command("git", "log", "--oneline", "-50", "--no-decorate")
		cmd.Dir = cfg.WorkDir
		if outBytes, err := cmd.Output(); err == nil {
			gitLog = strings.TrimSpace(string(outBytes))
		}

		// Get git diff stats to see what changed
		var gitDiffStats string
		cmd = exec.Command("git", "diff", "--stat", "HEAD~50..HEAD")
		cmd.Dir = cfg.WorkDir
		if outBytes, err := cmd.Output(); err == nil {
			gitDiffStats = strings.TrimSpace(string(outBytes))
		}

		// Build verification prompt with closed tasks and code changes
		var promptBuilder strings.Builder
		promptBuilder.WriteString(fmt.Sprintf("You are verifying epic %s after all leaf tasks have been completed.\n\n", cfg.Epic))
		promptBuilder.WriteString(fmt.Sprintf("# %s\n\n", setup.epicPromptData.Title))
		promptBuilder.WriteString(fmt.Sprintf("%s\n\n", setup.epicPromptData.Description))
		promptBuilder.WriteString("---\n\n")
		promptBuilder.WriteString("## Verification Task\n\n")
		promptBuilder.WriteString("Verify the epic is fully implemented by:\n")
		promptBuilder.WriteString("1. Reviewing all closed tasks under this epic\n")
		promptBuilder.WriteString("2. Reviewing code changes made during implementation\n")
		promptBuilder.WriteString("3. Checking that all requirements from the epic description have been met\n")
		promptBuilder.WriteString("4. Ensuring implementation is consistent and complete\n")
		promptBuilder.WriteString("5. Verifying code quality and testing standards are met\n")
		promptBuilder.WriteString("6. Checking if documentation needs updates\n\n")

		if len(allChildren) > 0 {
			promptBuilder.WriteString("## Closed Tasks\n\n")
			closedCount := 0
			for _, child := range allChildren {
				if child.Status == "closed" {
					promptBuilder.WriteString(fmt.Sprintf("- **%s**: %s\n", child.ID, child.Title))
					closedCount++
				}
			}
			if closedCount == 0 {
				promptBuilder.WriteString("(No closed tasks found - this may indicate an issue)\n")
			}
			promptBuilder.WriteString("\n")
			promptBuilder.WriteString("Review each closed task to understand what was implemented:\n")
			for _, child := range allChildren {
				if child.Status == "closed" {
					promptBuilder.WriteString(fmt.Sprintf("- Run `bd show %s` to see details of task \"%s\"\n", child.ID, child.Title))
				}
			}
			promptBuilder.WriteString("\n")
		}

		if gitLog != "" {
			promptBuilder.WriteString("## Recent Code Changes\n\n")
			promptBuilder.WriteString("Recent commit history:\n```\n")
			// Limit to first 30 lines to avoid prompt bloat
			lines := strings.Split(gitLog, "\n")
			if len(lines) > 30 {
				promptBuilder.WriteString(strings.Join(lines[:30], "\n"))
				promptBuilder.WriteString(fmt.Sprintf("\n... (%d more commits)\n", len(lines)-30))
			} else {
				promptBuilder.WriteString(gitLog)
			}
			promptBuilder.WriteString("\n```\n\n")
		}

		if gitDiffStats != "" {
			promptBuilder.WriteString("## Files Changed\n\n")
			promptBuilder.WriteString("```\n")
			promptBuilder.WriteString(gitDiffStats)
			promptBuilder.WriteString("\n```\n\n")
		}

		promptBuilder.WriteString("## Actions\n\n")
		promptBuilder.WriteString("After your review:\n\n")
		promptBuilder.WriteString(fmt.Sprintf("1. **If everything is complete and correct**: Close the epic with `bd close %s`\n\n", cfg.Epic))
		promptBuilder.WriteString("2. **If you find gaps or improvements needed**: Create follow-up beads for each issue:\n")
		promptBuilder.WriteString("   ```bash\n")
		promptBuilder.WriteString(fmt.Sprintf("   bd create \"<description of gap/improvement>\" --type task --parent %s\n", cfg.Epic))
		promptBuilder.WriteString("   ```\n")
		promptBuilder.WriteString("   Be specific about what needs to be done. Review the code changes and closed tasks to identify:\n")
		promptBuilder.WriteString("   - Missing functionality\n")
		promptBuilder.WriteString("   - Incomplete implementations\n")
		promptBuilder.WriteString("   - Code quality issues\n")
		promptBuilder.WriteString("   - Missing tests\n")
		promptBuilder.WriteString("   - Documentation gaps\n")
		promptBuilder.WriteString("   - Edge cases not handled\n\n")
		promptBuilder.WriteString("3. **If you have questions**: Create question beads:\n")
		promptBuilder.WriteString("   ```bash\n")
		promptBuilder.WriteString(fmt.Sprintf("   bd create \"Question: <your question>\" --type task --label needs-human --parent %s\n", cfg.Epic))
		promptBuilder.WriteString("   ```\n\n")
		promptBuilder.WriteString("**Important**: Do not close the epic if you create any follow-up beads. Only close it when you are confident the epic is fully implemented.\n")

		verificationPrompt := promptBuilder.String()

		// Run opus verification
		var opts []Option
		var verificationFormatter *LogFormatter
		if cfg.AgentTimeout > 0 {
			opts = append(opts, WithTimeout(cfg.AgentTimeout))
		}
		if !cfg.Verbose {
			verificationFormatter = NewLogFormatter(setup.out, false)
			opts = append(opts, WithStdoutWriter(verificationFormatter))
		} else if setup.out != os.Stdout {
			opts = append(opts, WithStdoutWriter(setup.out))
		}

		opusResult, err := RunAgentOpus(ctx, cfg.WorkDir, verificationPrompt, opts...)
		if err != nil {
			_, _ = fmt.Fprintf(setup.out, "Opus verification failed to run: %v\n", err)
		} else {
			// Print summary if formatter was used
			if verificationFormatter != nil && !cfg.Verbose {
				if summaryStr := verificationFormatter.Summary(); summaryStr != "" {
					_, _ = fmt.Fprintf(setup.out, "%s\n", summaryStr)
				}
			}
			_, _ = fmt.Fprintf(setup.out, "\nOpus verification completed (exit code %d, duration %s)\n", opusResult.ExitCode, formatDuration(opusResult.Duration))
		}
	}
}

// writeFinalEpicStatus writes the final summary for epic orchestrator.
func writeFinalEpicStatus(cfg LoopConfig, setup *epicOrchestratorSetup, loopStart time.Time) {
	setup.summary.Duration = time.Since(loopStart)
	if setup.summary.StopReason == StopNormal && setup.summary.Failed == 0 && setup.summary.TimedOut == 0 {
		setup.summary.StopReason = StopNormal
	}

	// Count remaining beads for summary
	remainingBeads := 0
	if setup.summary.StopReason == StopNormal {
		// Query one more time to see if any remain
		remaining, err := setup.fetchChildren()
		if err == nil {
			remainingBeads = len(remaining)
		}
	}

	// Print final summary
	_, _ = fmt.Fprintf(setup.out, "\n%s\n", formatSummary(setup.summary, remainingBeads))
}

// runEpicOrchestrator orchestrates epic leaf tasks sequentially, then runs opus verification.
// This is invoked when --epic flag is set (and --bead is not set).
// It queries 'bd ready --parent <epic-id>' after each completion to get fresh state
// and handle newly ready leaves, continuing until all leaves are done.
func runEpicOrchestrator(ctx context.Context, cfg LoopConfig) (*RunSummary, error) {
	ctx, setup, cleanup, err := setupEpicOrchestrator(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	loopStart := time.Now()

	// Main loop: query for ready leaves, process first one, repeat until none remain
	iteration := 0
	for {
		shouldContinue, err := processEpicIteration(ctx, cfg, setup, loopStart, iteration)
		if err != nil {
			return setup.summary, err
		}
		if !shouldContinue {
			break
		}
		iteration++
	}

	// If all children completed successfully, run opus verification
	runOpusVerification(ctx, cfg, setup)

	// Write final status and summary
	writeFinalEpicStatus(cfg, setup, loopStart)

	return setup.summary, nil
}
