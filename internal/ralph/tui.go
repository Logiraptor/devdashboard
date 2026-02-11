package ralph

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TUIModel is the Bubble Tea model for ralph TUI
type TUIModel struct {
	config      LoopConfig
	// traceView   *TraceViewModel // Will be created in separate task
	summary     *RunSummary
	status      string // Current status message
	err         error
	loopStarted bool
	loopDone    bool
	width       int
	height      int
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
	return &TUIModel{
		config:  cfg,
		summary: &RunSummary{},
	}
}

// Init implements tea.Model
func (m *TUIModel) Init() tea.Cmd {
	// Return command to start the loop in background
	return m.startLoopCmd()
}

// Update implements tea.Model
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Update trace view size if exists
		// Will be implemented when TraceViewModel is created
	case LoopStartedMsg:
		m.loopStarted = true
		m.status = "Loop started"
	case IterationStartMsg:
		m.status = fmt.Sprintf("Working on %s: %s", msg.BeadID, msg.BeadTitle)
	case IterationEndMsg:
		// Update summary counters based on outcome
		m.summary.Iterations++
		switch msg.Outcome {
		case OutcomeSuccess:
			m.summary.Succeeded++
		case OutcomeQuestion:
			m.summary.Questions++
		case OutcomeFailure:
			m.summary.Failed++
		case OutcomeTimeout:
			m.summary.TimedOut++
		}
	case LoopEndMsg:
		m.loopDone = true
		m.summary = msg.Summary
		m.status = fmt.Sprintf("Complete: %s", msg.StopReason)
	case LoopErrorMsg:
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

// View implements tea.Model
func (m *TUIModel) View() string {
	// Placeholder - will be enhanced with trace view
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	return fmt.Sprintf("Ralph Loop\nStatus: %s\n", m.status)
}

// startLoopCmd returns a command that runs the loop in background
func (m *TUIModel) startLoopCmd() tea.Cmd {
	return func() tea.Msg {
		// This will be implemented to run the actual loop
		// and send messages back via a channel
		return LoopStartedMsg{Epic: m.config.Epic}
	}
}
