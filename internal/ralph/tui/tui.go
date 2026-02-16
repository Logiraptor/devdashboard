// Package tui provides a terminal user interface for the ralph agent loop.
package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/ralph"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea model for ralph TUI.
type Model struct {
	core           *ralph.Core
	multiAgentView *MultiAgentView
	styles         Styles
	summary        summary
	status         string
	err            error
	loopStarted    bool
	loopDone       bool
	width          int
	height         int
	program        *tea.Program
	ctx            context.Context
	cancel         context.CancelFunc
	loopStart      time.Time

	// Track which bead each tool call belongs to
	toolToBeadMap map[string]string // tool call ID → bead ID
	currentBead   string            // Most recently started bead

	// Failure tracking for display
	lastFailure *ralph.BeadResult

	mu sync.Mutex // Protects fields updated from observer goroutine
}

// summary tracks aggregate results
type summary struct {
	Iterations int
	Succeeded  int
	Questions  int
	Failed     int
	TimedOut   int
	Duration   time.Duration
}

// Compile-time interface compliance check
var _ tea.Model = (*Model)(nil)

// Message types for TUI updates
type (
	loopStartedMsg   struct{ RootBead string }
	beadStartMsg     struct{ Bead beads.Bead }
	beadCompleteMsg  struct{ Result ralph.BeadResult }
	loopEndMsg       struct{ Result *ralph.CoreResult }
	loopErrorMsg     struct{ Err error }
	durationTickMsg  struct{}
)

// toolEventMsg wraps a tool event for the TUI
type toolEventMsg struct {
	Event   ralph.ToolEvent
	Started bool
}

// Observer implements ralph.ProgressObserver and forwards events to the TUI.
type Observer struct {
	ralph.NoopObserver
	program *tea.Program
}

// OnLoopStart is called when the loop begins.
func (o *Observer) OnLoopStart(rootBead string) {
	if o.program != nil {
		o.program.Send(loopStartedMsg{RootBead: rootBead})
	}
}

// OnBeadStart is called when work begins on a bead.
func (o *Observer) OnBeadStart(bead beads.Bead) {
	if o.program != nil {
		o.program.Send(beadStartMsg{Bead: bead})
	}
}

// OnBeadComplete is called when a bead finishes.
func (o *Observer) OnBeadComplete(result ralph.BeadResult) {
	if o.program != nil {
		o.program.Send(beadCompleteMsg{Result: result})
	}
}

// OnLoopEnd is called when the loop completes.
func (o *Observer) OnLoopEnd(result *ralph.CoreResult) {
	if o.program != nil {
		o.program.Send(loopEndMsg{Result: result})
	}
}

// OnToolStart is called when a tool call begins.
func (o *Observer) OnToolStart(event ralph.ToolEvent) {
	if o.program != nil {
		o.program.Send(toolEventMsg{Event: event, Started: true})
	}
}

// OnToolEnd is called when a tool call ends.
func (o *Observer) OnToolEnd(event ralph.ToolEvent) {
	if o.program != nil {
		o.program.Send(toolEventMsg{Event: event, Started: false})
	}
}

// NewModel creates a new TUI model for the given Core.
func NewModel(core *ralph.Core) *Model {
	styles := DefaultStyles()
	return &Model{
		core:           core,
		multiAgentView: NewMultiAgentView(),
		styles:         styles,
		toolToBeadMap:  make(map[string]string),
	}
}

// Run starts the TUI and runs the Core loop.
// This is the main entry point for running ralph with a TUI.
// If additionalObserver is provided, it will be combined with the TUI observer
// using MultiObserver so both receive progress updates.
func Run(ctx context.Context, core *ralph.Core, additionalObserver ralph.ProgressObserver) error {
	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create model
	m := NewModel(core)
	m.ctx = ctx
	m.cancel = cancel

	// Create program
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.program = p

	// Set up observer to forward events to TUI
	tuiObserver := &Observer{
		program: p,
	}

	// Combine TUI observer with additional observer if provided
	if additionalObserver != nil {
		core.Observer = ralph.NewMultiObserver(additionalObserver, tuiObserver)
	} else {
		core.Observer = tuiObserver
	}

	// Run Core in background goroutine
	go func() {
		m.loopStart = time.Now()

		// Start duration ticker (faster for smoother animation)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ticker.C:
					p.Send(durationTickMsg{})
				case <-ctx.Done():
					return
				}
			}
		}()

		// Run the core loop
		result, err := core.Run(ctx)
		if err != nil {
			p.Send(loopErrorMsg{Err: err})
			return
		}

		// Ensure loopEnd is sent even if observer didn't fire
		// (in case of early exit or error)
		if result != nil {
			p.Send(loopEndMsg{Result: result})
		}
	}()

	// Run TUI
	_, err := p.Run()
	return err
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve space for header/footer
		viewHeight := msg.Height - 6
		if viewHeight < 10 {
			viewHeight = 10
		}
		m.multiAgentView.SetSize(msg.Width, viewHeight)

	case loopStartedMsg:
		m.mu.Lock()
		m.loopStarted = true
		m.status = fmt.Sprintf("Loop started: %s", msg.RootBead)
		m.mu.Unlock()

	case beadStartMsg:
		m.mu.Lock()
		m.currentBead = msg.Bead.ID
		m.status = fmt.Sprintf("Working on %s: %s", msg.Bead.ID, msg.Bead.Title)
		m.mu.Unlock()

		// Start tracking this agent in the view
		m.multiAgentView.StartAgent(msg.Bead)

	case beadCompleteMsg:
		m.mu.Lock()
		r := msg.Result
		switch r.Outcome {
		case ralph.OutcomeSuccess:
			m.summary.Succeeded++
		case ralph.OutcomeFailure:
			m.summary.Failed++
			m.lastFailure = &r
		case ralph.OutcomeTimeout:
			m.summary.TimedOut++
			m.lastFailure = &r
		case ralph.OutcomeQuestion:
			m.summary.Questions++
		}
		m.summary.Iterations++

		// Clear current bead if this was it
		if m.currentBead == r.Bead.ID {
			m.currentBead = ""
		}
		m.mu.Unlock()

		// Update the agent view
		status := "success"
		switch r.Outcome {
		case ralph.OutcomeFailure:
			status = "failed"
		case ralph.OutcomeTimeout:
			status = "timeout"
		case ralph.OutcomeQuestion:
			status = "question"
		}
		m.multiAgentView.CompleteAgent(r.Bead.ID, status)

	case toolEventMsg:
		// Route tool event to the appropriate agent
		m.mu.Lock()
		beadID := m.currentBead
		if beadID == "" {
			// Fall back to looking up from tool ID
			beadID = m.toolToBeadMap[msg.Event.ID]
		}
		if msg.Started && beadID != "" {
			// Track this tool call for the end event
			m.toolToBeadMap[msg.Event.ID] = beadID
		} else if !msg.Started {
			// Clean up tracking
			delete(m.toolToBeadMap, msg.Event.ID)
		}
		m.mu.Unlock()

		if beadID != "" {
			m.multiAgentView.AddToolEvent(beadID, msg.Event.Name, msg.Started, msg.Event.Attributes)
		}

	case loopEndMsg:
		m.mu.Lock()
		m.loopDone = true
		if msg.Result != nil {
			m.summary.Duration = msg.Result.Duration
			m.summary.Succeeded = msg.Result.Succeeded
			m.summary.Questions = msg.Result.Questions
			m.summary.Failed = msg.Result.Failed
			m.summary.TimedOut = msg.Result.TimedOut
		}
		if m.summary.Iterations == 0 {
			m.status = "No beads available"
		} else {
			m.status = "Complete"
		}
		m.mu.Unlock()

	case loopErrorMsg:
		m.err = msg.Err
		return m, tea.Quit

	case durationTickMsg:
		if m.loopStarted && !m.loopDone {
			m.mu.Lock()
			m.summary.Duration = time.Since(m.loopStart)
			m.mu.Unlock()
			// Update running agent durations
			m.multiAgentView.UpdateDuration()
		}
	}

	return m, nil
}

// View implements tea.Model
func (m *Model) View() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v\n", m.err))
	}

	var b strings.Builder

	// Header with logo
	header := renderHeader(m.styles, m.core.RootBead, m.width)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Multi-agent view
	b.WriteString(m.multiAgentView.View())
	b.WriteString("\n\n")

	// Status bar
	m.mu.Lock()
	loopDone := m.loopDone
	loopStarted := m.loopStarted
	duration := m.summary.Duration
	m.mu.Unlock()

	statusBar := renderStatusBar(m.styles, loopStarted, loopDone, m.multiAgentView.Summary(), duration, m.width)
	b.WriteString(statusBar)

	// Quit hint
	if loopDone {
		b.WriteString("\n")
		b.WriteString(m.styles.Muted.Render("Press q to quit"))
	}

	return b.String()
}

// renderHeader renders the top header
func renderHeader(styles Styles, rootBead string, width int) string {
	// Stylized header
	title := styles.Title.Render("⚡ RALPH")
	subtitle := ""
	if rootBead != "" {
		subtitle = " " + styles.Subtitle.Render("→ "+rootBead)
	}

	return title + subtitle
}

// renderStatusBar renders the bottom status bar
func renderStatusBar(styles Styles, started, done bool, summary string, duration time.Duration, width int) string {
	var parts []string

	// Status indicator
	if done {
		parts = append(parts, styles.Success.Render("✓ Complete"))
	} else if started {
		parts = append(parts, styles.Status.Render("● Running"))
	} else {
		parts = append(parts, styles.Muted.Render("○ Waiting"))
	}

	// Summary from multi-agent view
	if summary != "" {
		parts = append(parts, styles.Status.Render(summary))
	}

	// Duration
	if started && duration > 0 {
		parts = append(parts, styles.Muted.Render(ralph.FormatDuration(duration)))
	}

	return strings.Join(parts, " │ ")
}
