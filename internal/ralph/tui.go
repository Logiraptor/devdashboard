package ralph

import (
	"context"
	"fmt"
	"strings"
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
}

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
	BeadID   string
	Outcome  Outcome
	Duration time.Duration
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

// NewTUIModel creates a new TUI model with the given config
func NewTUIModel(cfg LoopConfig) *TUIModel {
	styles := DefaultStyles()
	return &TUIModel{
		config:       cfg,
		traceView:    NewTraceViewModel(styles),
		traceEmitter: NewLocalTraceEmitter(),
		styles:       styles,
		summary:      &RunSummary{},
	}
}

// GetTraceEmitter returns the trace emitter for loop integration
func (m *TUIModel) GetTraceEmitter() *LocalTraceEmitter {
	return m.traceEmitter
}

// SetProgram sets the tea.Program for sending messages
// Must be called after tea.NewProgram(). This will start the loop.
func (m *TUIModel) SetProgram(p *tea.Program) {
	m.program = p
	m.traceEmitter.SetProgram(p)
	// Start the loop now that we have the program reference
	if !m.loopStartedFlag {
		m.loopStartedFlag = true
		go func() {
			m.runLoopWithMessages()
		}()
	}
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
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			cmds = append(cmds, m.traceView.Update(msg))
		case "k", "up":
			cmds = append(cmds, m.traceView.Update(msg))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve space for header/footer
		traceHeight := msg.Height - 4
		if traceHeight < 10 {
			traceHeight = 10
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
		m.status = fmt.Sprintf("Working on %s: %s", msg.BeadID, msg.BeadTitle)

	case IterationEndMsg:
		// Update summary counters
		switch msg.Outcome {
		case OutcomeSuccess:
			m.summary.Succeeded++
		case OutcomeFailure:
			m.summary.Failed++
		case OutcomeTimeout:
			m.summary.TimedOut++
		case OutcomeQuestion:
			m.summary.Questions++
		}
		m.summary.Iterations++

	case LoopEndMsg:
		m.loopDone = true
		m.summary = msg.Summary
		m.status = fmt.Sprintf("Complete: %s", msg.StopReason)

	case LoopErrorMsg:
		m.err = msg.Err
		return m, tea.Quit
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
	statusLine := m.styles.Status.Render(m.status)
	if m.summary.Iterations > 0 {
		stats := fmt.Sprintf(" | %d done, %d failed",
			m.summary.Succeeded, m.summary.Failed)
		statusLine += m.styles.Muted.Render(stats)
	}
	b.WriteString(statusLine)

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
	ctx := context.Background()

	// Apply wall-clock timeout
	wallTimeout := cfg.Timeout
	if wallTimeout <= 0 {
		wallTimeout = DefaultWallClockTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wallTimeout)
	defer cancel()

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

	// Set up picker
	picker := &BeadPicker{WorkDir: cfg.WorkDir, Epic: cfg.Epic}

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
		bead, err := picker.Next()
		if err != nil {
			if m.program != nil {
				m.program.Send(LoopErrorMsg{Err: err})
			}
			return
		}
		if bead == nil {
			summary.StopReason = StopNormal
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
		outcome, _ := Assess(cfg.WorkDir, bead.ID, result)

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

		// End iteration
		durationMs := time.Since(iterStart).Milliseconds()
		emitter.EndIteration(spanID, outcome.String(), durationMs)

		if m.program != nil {
			m.program.Send(IterationEndMsg{
				BeadID:   bead.ID,
				Outcome:  outcome,
				Duration: time.Since(iterStart),
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
