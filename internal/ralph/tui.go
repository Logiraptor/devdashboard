package ralph

import (
	"fmt"
	"strings"
	"time"

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

// Init implements tea.Model
func (m *TUIModel) Init() tea.Cmd {
	// Return command to start the loop in background
	return m.startLoopCmd()
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

// startLoopCmd returns a command that runs the loop in background
func (m *TUIModel) startLoopCmd() tea.Cmd {
	return func() tea.Msg {
		// This will be implemented to run the actual loop
		// and send messages back via a channel
		return LoopStartedMsg{Epic: m.config.Epic}
	}
}
