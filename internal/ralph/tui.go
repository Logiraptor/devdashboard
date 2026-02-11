package ralph

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"devdeploy/internal/beads"
	tea "github.com/charmbracelet/bubbletea"
)

// TUIModel is the Bubble Tea model for ralph TUI
type TUIModel struct {
	config       LoopConfig
	traceView    *TraceViewModel
	traceEmitter *LocalTraceEmitter
	styles       RalphStyles
	summary      *RunSummary
	status       string // Current status message
	err          error
	loopStarted  bool
	loopDone     bool
	width        int
	height       int
	program      *tea.Program // Set after tea.NewProgram() for sending messages
	loopStartedFlag bool // Track if loop has been started
	currentIter  int    // Current iteration number (1-indexed)
	maxIter      int    // Maximum iterations
	ctx          context.Context // Context for cancellation
	cancel       context.CancelFunc // Cancel function
	loopStartTime time.Time // When the loop started

	// Failure tracking for display
	lastFailure *IterationEndMsg // Last failed iteration for display
}

// Compile-time interface compliance check
var _ tea.Model = (*TUIModel)(nil)

// Message types for loop communication
type LoopStartedMsg struct {
	TraceID string
	Epic    string
}

type IterationStartMsg struct {
	BeadID    string
	BeadTitle string
	IterNum   int
}

type IterationEndMsg struct {
	BeadID       string
	Outcome      Outcome
	Duration     time.Duration
	ChatID       string // Chat session ID from the agent (for debugging failures)
	ErrorMessage string // Error message from the agent, if any
	ExitCode     int    // Agent process exit code
	Stderr       string // Stderr output from the agent (for debugging when ErrorMessage is empty)
}

type ToolCallStartMsg struct {
	SpanID   string
	ToolName string
	Attrs    map[string]string
}

type ToolCallEndMsg struct {
	SpanID string
	Attrs  map[string]string
}

type LoopEndMsg struct {
	Summary    *RunSummary
	StopReason StopReason
}

type LoopErrorMsg struct {
	Err error
}

type DurationTickMsg struct{}

// NewTUIModel creates a new TUI model with the given config
func NewTUIModel(cfg LoopConfig) *TUIModel {
	styles := DefaultStyles()
	return &TUIModel{
		config:       cfg,
		traceView:    NewTraceViewModel(styles),
		traceEmitter: NewLocalTraceEmitter(),
		styles:       styles,
		summary:      &RunSummary{},
		maxIter:      cfg.MaxIterations,
	}
}

// TraceEmitter returns the trace emitter for loop integration
func (m *TUIModel) TraceEmitter() *LocalTraceEmitter {
	return m.traceEmitter
}

// Err returns any error that occurred during the loop
func (m *TUIModel) Err() error {
	return m.err
}

// Summary returns the run summary
func (m *TUIModel) Summary() *RunSummary {
	return m.summary
}

// SetProgram sets the tea.Program for sending messages
// Must be called after tea.NewProgram(). This will start the loop.
func (m *TUIModel) SetProgram(p *tea.Program) {
	m.program = p
	m.traceEmitter.SetProgram(p)
	// Start the loop now that we have the program reference
	if !m.loopStartedFlag {
		m.loopStartedFlag = true
		m.loopStartTime = time.Now()
		go func() {
			m.runLoopWithMessages()
		}()
	}
}

// SetContext sets the context for cancellation
func (m *TUIModel) SetContext(ctx context.Context, cancel context.CancelFunc) {
	m.ctx = ctx
	m.cancel = cancel
}

// Init implements tea.Model
func (m *TUIModel) Init() tea.Cmd {
	// Don't start loop here - wait for SetProgram to be called
	// The loop will start when SetProgram is called
	return nil
}

// Update implements tea.Model
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			// Cancel context if set
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "ctrl+c":
			// Cancel context if set
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "j", "down":
			cmds = append(cmds, m.traceView.Update(msg))
		case "k", "up":
			cmds = append(cmds, m.traceView.Update(msg))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve space for header/footer (header + status bar + quit hint)
		traceHeight := msg.Height - 4
		if traceHeight < 5 {
			traceHeight = 5 // Minimum height for very small terminals
		}
		m.traceView.SetSize(msg.Width, traceHeight)

	case TraceUpdateMsg:
		m.traceView.SetTrace(msg.Trace)

	case LoopStartedMsg:
		m.loopStarted = true
		m.status = fmt.Sprintf("Loop started: %s", msg.Epic)
		// Start listening for subsequent messages
		// Note: We'll need to store the channel reference, but for now
		// the loop will send messages directly via program.Send

	case IterationStartMsg:
		m.currentIter = msg.IterNum
		m.status = fmt.Sprintf("Working on %s: %s", msg.BeadID, msg.BeadTitle)

	case IterationEndMsg:
		// Update summary counters
		switch msg.Outcome {
		case OutcomeSuccess:
			m.summary.Succeeded++
		case OutcomeFailure:
			m.summary.Failed++
			// Track last failure for display
			msgCopy := msg
			m.lastFailure = &msgCopy
		case OutcomeTimeout:
			m.summary.TimedOut++
			// Track timeouts as failures too
			msgCopy := msg
			m.lastFailure = &msgCopy
		case OutcomeQuestion:
			m.summary.Questions++
		}
		m.summary.Iterations++

	case LoopEndMsg:
		m.loopDone = true
		m.summary = msg.Summary
		if msg.StopReason == StopNormal && m.summary.Iterations == 0 {
			m.status = "No beads available"
		} else {
			m.status = fmt.Sprintf("Complete: %s", msg.StopReason)
		}

	case LoopErrorMsg:
		m.err = msg.Err
		return m, tea.Quit

	case DurationTickMsg:
		// Update duration display (already updated by ticker goroutine)
		// This message just triggers a re-render
		if m.loopStarted && !m.loopDone {
			return m, nil
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m *TUIModel) View() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v\n", m.err))
	}

	var b strings.Builder

	// Header
	header := m.styles.Title.Render("Ralph Loop")
	if m.config.Epic != "" {
		header += " " + m.styles.Subtitle.Render(m.config.Epic)
	}
	b.WriteString(header)
	b.WriteString("\n")

	// Trace view
	b.WriteString(m.traceView.View())
	b.WriteString("\n")

	// Status bar
	// Max 5 parts: status (1) + iter (1) + status msg (1) + stats (1) + duration (1)
	statusParts := make([]string, 0, 5)
	
	// Show running/completed status
	if m.loopDone {
		statusParts = append(statusParts, m.styles.Success.Render("✓ Completed"))
	} else if m.loopStarted {
		statusParts = append(statusParts, m.styles.Status.Render("● Running"))
	} else {
		statusParts = append(statusParts, m.styles.Muted.Render("Waiting..."))
	}
	
	// Show iteration count if loop has started
	if m.loopStarted && m.maxIter > 0 {
		iterInfo := fmt.Sprintf("[%d/%d]", m.currentIter, m.maxIter)
		statusParts = append(statusParts, m.styles.Muted.Render(iterInfo))
	}
	
	// Show current status message
	if m.status != "" {
		statusParts = append(statusParts, m.styles.Status.Render(m.status))
	}
	
	// Show summary stats if we have iterations
	if m.summary.Iterations > 0 {
		stats := fmt.Sprintf("%d done, %d failed",
			m.summary.Succeeded, m.summary.Failed)
		statusParts = append(statusParts, m.styles.Muted.Render(stats))
	}
	
	// Show duration if loop is running or done
	if m.loopStarted && m.summary.Duration > 0 {
		durationStr := formatDuration(m.summary.Duration)
		statusParts = append(statusParts, m.styles.Muted.Render(durationStr))
	}
	
	statusLine := strings.Join(statusParts, " | ")
	b.WriteString(statusLine)

	// Show failure details if there was a failure
	if m.lastFailure != nil && (m.lastFailure.Outcome == OutcomeFailure || m.lastFailure.Outcome == OutcomeTimeout) {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("Last Failure:"))
		b.WriteString("\n")
		
		// Bead ID
		b.WriteString(fmt.Sprintf("  Bead: %s", m.lastFailure.BeadID))
		
		// Exit code
		if m.lastFailure.ExitCode != 0 {
			b.WriteString(fmt.Sprintf(" (exit code %d)", m.lastFailure.ExitCode))
		}
		b.WriteString("\n")
		
		// Chat ID - important for debugging
		if m.lastFailure.ChatID != "" {
			b.WriteString(fmt.Sprintf("  ChatID: %s\n", m.styles.Muted.Render(m.lastFailure.ChatID)))
		}
		
		// Error message
		if m.lastFailure.ErrorMessage != "" {
			errMsg := m.lastFailure.ErrorMessage
			// Truncate long error messages
			if len(errMsg) > 100 {
				errMsg = errMsg[:97] + "..."
			}
			b.WriteString(fmt.Sprintf("  Error: %s\n", m.styles.Error.Render(errMsg)))
		} else if m.lastFailure.Stderr != "" {
			// Show stderr as fallback when no structured error message
			stderrLines := strings.Split(strings.TrimSpace(m.lastFailure.Stderr), "\n")
			// Show last 3 lines of stderr (most likely to be relevant)
			start := len(stderrLines) - 3
			if start < 0 {
				start = 0
			}
			b.WriteString("  Stderr:\n")
			for _, line := range stderrLines[start:] {
				if line = strings.TrimSpace(line); line != "" {
					// Truncate long lines
					if len(line) > 100 {
						line = line[:97] + "..."
					}
					b.WriteString(fmt.Sprintf("    %s\n", m.styles.Error.Render(line)))
				}
			}
		} else {
			// No error info at all
			b.WriteString(fmt.Sprintf("  %s\n", m.styles.Muted.Render("No error details available")))
		}
	}

	// Quit hint
	if m.loopDone {
		b.WriteString("\n")
		b.WriteString(m.styles.Muted.Render("Press q to quit"))
	}

	return b.String()
}


// runLoopWithMessages executes the loop and sends messages via program.Send()
func (m *TUIModel) runLoopWithMessages() {
	cfg := m.config
	
	// Use provided context or create a new one
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Apply wall-clock timeout
	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallTimeout)
	defer cancel()
	
	// Start a ticker to update duration in real-time
	durationTicker := time.NewTicker(1 * time.Second)
	defer durationTicker.Stop()
	
	go func() {
		for {
			select {
			case <-durationTicker.C:
				if m.loopStarted && !m.loopDone && m.program != nil {
					m.summary.Duration = time.Since(m.loopStartTime)
					m.program.Send(DurationTickMsg{})
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start loop trace
	emitter := m.traceEmitter
	epic := cfg.Epic
	if epic == "" {
		epic = "default"
	}
	traceID := emitter.StartLoop("composer-1", epic, cfg.WorkDir, cfg.MaxIterations)

	if m.program != nil {
		m.program.Send(LoopStartedMsg{TraceID: traceID, Epic: epic})
	}

	// Use concurrent execution if Concurrency > 1, otherwise sequential
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	if concurrency > 1 {
		m.runConcurrentLoop(ctx, cfg, emitter)
	} else {
		m.runSequentialLoop(ctx, cfg, emitter)
	}
}

// runSequentialLoop runs beads one at a time (original behavior)
func (m *TUIModel) runSequentialLoop(ctx context.Context, cfg LoopConfig, emitter *LocalTraceEmitter) {
	// Set up picker - respect TargetBead if set
	var pickNext func() (*beads.Bead, error)
	if cfg.TargetBead != "" {
		// When TargetBead is set, return that bead once then nil
		once := false
		pickNext = func() (*beads.Bead, error) {
			if once {
				return nil, nil // Signal no more beads
			}
			once = true
			return fetchTargetBead(cfg.WorkDir, cfg.TargetBead)
		}
	} else {
		picker := &BeadPicker{WorkDir: cfg.WorkDir, Epic: cfg.Epic}
		pickNext = picker.Next
	}

	summary := &RunSummary{}
	loopStart := time.Now()

	for i := 0; i < cfg.MaxIterations; i++ {
		// Check context
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				summary.StopReason = StopWallClock
			} else {
				summary.StopReason = StopContextCancelled
			}
			break
		}

		// Pick next bead
		bead, err := pickNext()
		if err != nil {
			if m.program != nil {
				m.program.Send(LoopErrorMsg{Err: err})
			}
			return
		}
		if bead == nil {
			summary.StopReason = StopNormal
			if m.program != nil {
				m.program.Send(LoopEndMsg{
					Summary:    summary,
					StopReason: StopNormal,
				})
			}
			break
		}

		// Start iteration
		iterStart := time.Now()
		spanID := emitter.StartIteration(bead.ID, bead.Title, i+1)
		emitter.SetParent(spanID)

		if m.program != nil {
			m.program.Send(IterationStartMsg{
				BeadID:    bead.ID,
				BeadTitle: bead.Title,
				IterNum:   i + 1,
			})
		}

		// Execute agent (using existing executor)
		// Pass trace emitter for tool call tracking
		result, err := m.executeAgent(ctx, bead, emitter)
		if err != nil {
			if m.program != nil {
				m.program.Send(LoopErrorMsg{Err: err})
			}
			return
		}

		// Assess outcome
		outcome, _ := Assess(cfg.WorkDir, bead.ID, result, nil)

		// Update summary
		summary.Iterations++
		switch outcome {
		case OutcomeSuccess:
			summary.Succeeded++
		case OutcomeFailure:
			summary.Failed++
		case OutcomeTimeout:
			summary.TimedOut++
		case OutcomeQuestion:
			summary.Questions++
		}

		// End iteration with failure details in trace
		durationMs := time.Since(iterStart).Milliseconds()
		extraAttrs := map[string]string{}
		if result.ChatID != "" {
			extraAttrs["chat_id"] = result.ChatID
		}
		if result.ErrorMessage != "" {
			extraAttrs["error"] = result.ErrorMessage
		}
		if result.ExitCode != 0 {
			extraAttrs["exit_code"] = fmt.Sprintf("%d", result.ExitCode)
		}
		emitter.EndIterationWithAttrs(spanID, outcome.String(), durationMs, extraAttrs)

		if m.program != nil {
			m.program.Send(IterationEndMsg{
				BeadID:       bead.ID,
				Outcome:      outcome,
				Duration:     time.Since(iterStart),
				ChatID:       result.ChatID,
				ErrorMessage: result.ErrorMessage,
				ExitCode:     result.ExitCode,
				Stderr:       result.Stderr,
			})
		}
	}

	// End loop
	summary.Duration = time.Since(loopStart)
	emitter.EndLoop(summary.StopReason.String(), summary.Iterations, summary.Succeeded, summary.Failed)

	if m.program != nil {
		m.program.Send(LoopEndMsg{Summary: summary, StopReason: summary.StopReason})
	}
}

// runConcurrentLoop runs multiple beads in parallel, respecting concurrency limit
func (m *TUIModel) runConcurrentLoop(ctx context.Context, cfg LoopConfig, emitter *LocalTraceEmitter) {
	// Initialize worktree manager for isolation
	wtMgr, err := NewWorktreeManager(cfg.WorkDir)
	if err != nil {
		if m.program != nil {
			m.program.Send(LoopErrorMsg{Err: fmt.Errorf("creating worktree manager: %w", err)})
		}
		return
	}

	// Fetch beads to work on
	var readyBeads []beads.Bead

	// If TargetBead is set, work on just that bead
	if cfg.TargetBead != "" {
		targetBead, fetchErr := fetchTargetBead(cfg.WorkDir, cfg.TargetBead)
		if fetchErr != nil {
			if m.program != nil {
				m.program.Send(LoopErrorMsg{Err: fmt.Errorf("fetching target bead: %w", fetchErr)})
			}
			return
		}
		readyBeads = []beads.Bead{*targetBead}
	} else if cfg.Epic != "" {
		// Fetch ready children of epic
		readyBeads, err = FetchEpicChildren(nil, cfg.WorkDir, cfg.Epic)
	} else {
		// Fetch all ready beads
		runBD := func(dir string, args ...string) ([]byte, error) {
			cmd := exec.Command("bd", args...)
			cmd.Dir = dir
			return cmd.Output()
		}
		out, bdErr := runBD(cfg.WorkDir, "ready", "--json")
		if bdErr != nil {
			err = fmt.Errorf("bd ready: %w", bdErr)
		} else {
			readyBeads, err = parseReadyBeads(out)
		}
	}
	
	if err != nil {
		if m.program != nil {
			m.program.Send(LoopErrorMsg{Err: fmt.Errorf("fetching ready beads: %w", err)})
		}
		return
	}

	if len(readyBeads) == 0 {
		summary := &RunSummary{
			StopReason: StopNormal,
			Duration:   time.Since(m.loopStartTime),
		}
		if m.program != nil {
			m.program.Send(LoopEndMsg{Summary: summary, StopReason: StopNormal})
		}
		return
	}

	// Limit to MaxIterations
	if len(readyBeads) > cfg.MaxIterations {
		readyBeads = readyBeads[:cfg.MaxIterations]
	}

	// Dispatch beads in parallel, respecting concurrency limit
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	// Create a semaphore channel to limit concurrency
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	summary := &RunSummary{}
	summaryMu := &sync.Mutex{}
	loopStart := time.Now()

	for i := range readyBeads {
		bead := &readyBeads[i]
		
		// Check context before starting new goroutine
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(beadIdx int, bead *beads.Bead) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Check context again after acquiring semaphore
			if ctx.Err() != nil {
				return
			}

			// Create worktree for this bead
			worktreePath, _, err := wtMgr.CreateWorktree(bead.ID)
			if err != nil {
				summaryMu.Lock()
				summary.Failed++
				summary.Iterations++
				summaryMu.Unlock()
				if m.program != nil {
					m.program.Send(IterationEndMsg{
						BeadID:       bead.ID,
						Outcome:      OutcomeFailure,
						ErrorMessage: fmt.Sprintf("worktree creation failed: %v", err),
					})
				}
				return
			}
			defer func() {
				if err := wtMgr.RemoveWorktree(worktreePath); err != nil {
					// Best effort cleanup, don't fail on error
				}
			}()

			// Start iteration
			iterStart := time.Now()
			spanID := emitter.StartIteration(bead.ID, bead.Title, beadIdx+1)
			emitter.SetParent(spanID)

			if m.program != nil {
				m.program.Send(IterationStartMsg{
					BeadID:    bead.ID,
					BeadTitle: bead.Title,
					IterNum:   beadIdx + 1,
				})
			}

			// Fetch prompt data (use original workdir for beads state)
			promptData, err := FetchPromptData(nil, cfg.WorkDir, bead.ID)
			if err != nil {
				summaryMu.Lock()
				summary.Failed++
				summary.Iterations++
				summaryMu.Unlock()
				if m.program != nil {
					m.program.Send(IterationEndMsg{
						BeadID:       bead.ID,
						Outcome:      OutcomeFailure,
						ErrorMessage: fmt.Sprintf("prompt fetch failed: %v", err),
					})
				}
				return
			}

			// Render prompt
			prompt, err := RenderPrompt(promptData)
			if err != nil {
				summaryMu.Lock()
				summary.Failed++
				summary.Iterations++
				summaryMu.Unlock()
				if m.program != nil {
					m.program.Send(IterationEndMsg{
						BeadID:       bead.ID,
						Outcome:      OutcomeFailure,
						ErrorMessage: fmt.Sprintf("prompt render failed: %v", err),
					})
				}
				return
			}

			// Execute agent in worktree (use worktree path for isolation)
			var opts []Option
			if cfg.AgentTimeout > 0 {
				opts = append(opts, WithTimeout(cfg.AgentTimeout))
			}
			// Create trace writer that uses local emitter
			traceWriter := NewLocalTraceWriter(emitter)
			opts = append(opts, WithStdoutWriter(traceWriter))

			result, err := RunAgent(ctx, worktreePath, prompt, opts...)
			if err != nil {
				summaryMu.Lock()
				summary.Failed++
				summary.Iterations++
				summaryMu.Unlock()
				if m.program != nil {
					m.program.Send(IterationEndMsg{
						BeadID:       bead.ID,
						Outcome:      OutcomeFailure,
						ErrorMessage: fmt.Sprintf("agent execution failed: %v", err),
					})
				}
				return
			}

			// Assess outcome (use original workdir for beads state)
			outcome, _ := Assess(cfg.WorkDir, bead.ID, result, nil)

			// Update summary atomically
			summaryMu.Lock()
			summary.Iterations++
			switch outcome {
			case OutcomeSuccess:
				summary.Succeeded++
			case OutcomeFailure:
				summary.Failed++
			case OutcomeTimeout:
				summary.TimedOut++
			case OutcomeQuestion:
				summary.Questions++
			}
			summaryMu.Unlock()

			// End iteration with failure details in trace
			durationMs := time.Since(iterStart).Milliseconds()
			extraAttrs := map[string]string{}
			if result.ChatID != "" {
				extraAttrs["chat_id"] = result.ChatID
			}
			if result.ErrorMessage != "" {
				extraAttrs["error"] = result.ErrorMessage
			}
			if result.ExitCode != 0 {
				extraAttrs["exit_code"] = fmt.Sprintf("%d", result.ExitCode)
			}
			emitter.EndIterationWithAttrs(spanID, outcome.String(), durationMs, extraAttrs)

			if m.program != nil {
				m.program.Send(IterationEndMsg{
					BeadID:       bead.ID,
					Outcome:      outcome,
					Duration:     time.Since(iterStart),
					ChatID:       result.ChatID,
					ErrorMessage: result.ErrorMessage,
					ExitCode:     result.ExitCode,
					Stderr:       result.Stderr,
				})
			}

			// Sync beads state (best-effort)
			syncFn := func() error {
				cmd := exec.Command("bd", "sync")
				cmd.Dir = cfg.WorkDir
				return cmd.Run()
			}
			if err := syncFn(); err != nil {
				// Best effort, don't fail on sync errors
			}
		}(i, bead)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Determine stop reason
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			summary.StopReason = StopWallClock
		} else {
			summary.StopReason = StopContextCancelled
		}
	} else {
		summary.StopReason = StopNormal
	}

	// End loop
	summary.Duration = time.Since(loopStart)
	emitter.EndLoop(summary.StopReason.String(), summary.Iterations, summary.Succeeded, summary.Failed)

	if m.program != nil {
		m.program.Send(LoopEndMsg{Summary: summary, StopReason: summary.StopReason})
	}
}

// fetchTargetBead fetches a specific bead by ID using bd show.
func fetchTargetBead(workDir, beadID string) (*beads.Bead, error) {
	showCmd := exec.Command("bd", "show", beadID, "--json")
	showCmd.Dir = workDir
	outBytes, err := showCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd show %s: %w", beadID, err)
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
		return nil, fmt.Errorf("bead %s not found", beadID)
	}
	e := showEntries[0]
	return &beads.Bead{
		ID:    e.ID,
		Title: e.Title,
	}, nil
}

// executeAgent runs the agent and tracks tool calls via trace emitter
func (m *TUIModel) executeAgent(ctx context.Context, bead *beads.Bead, emitter *LocalTraceEmitter) (*AgentResult, error) {
	cfg := m.config

	// Fetch prompt
	promptData, err := FetchPromptData(nil, cfg.WorkDir, bead.ID)
	if err != nil {
		return nil, err
	}

	prompt, err := RenderPrompt(promptData)
	if err != nil {
		return nil, err
	}

	// Create trace writer that uses local emitter
	traceWriter := NewLocalTraceWriter(emitter)

	// Run agent with trace writer
	var opts []Option
	if cfg.AgentTimeout > 0 {
		opts = append(opts, WithTimeout(cfg.AgentTimeout))
	}
	opts = append(opts, WithStdoutWriter(traceWriter))

	return RunAgent(ctx, cfg.WorkDir, prompt, opts...)
}
