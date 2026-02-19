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
	"devdeploy/internal/ui/textutil"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MaxToolEvents is the number of recent tool events to display
const MaxToolEvents = 6

// ToolEventDisplay represents a tool event for display
type ToolEventDisplay struct {
	Name      string
	Detail    string
	Started   time.Time
	Duration  time.Duration
	Completed bool
}

// Model is the Bubble Tea model for ralph TUI.
type Model struct {
	core      *ralph.Core
	styles    Styles
	status    string
	err       error
	loopStart time.Time

	// Bead info
	beadID    string
	beadTitle string

	// Iteration tracking
	iteration  int
	maxIter    int
	toolEvents []ToolEventDisplay // Ring buffer of recent tool events

	// State
	loopStarted bool
	loopDone    bool
	duration    time.Duration
	outcome     ralph.Outcome

	// UI dimensions
	width  int
	height int

	program *tea.Program
	ctx     context.Context
	cancel  context.CancelFunc

	mu sync.Mutex // Protects fields updated from observer goroutine
}

// Compile-time interface compliance check
var _ tea.Model = (*Model)(nil)

// Message types for TUI updates
type (
	loopStartedMsg     struct{ RootBead string }
	beadStartMsg       struct{ Bead beads.Bead }
	beadCompleteMsg    struct{ Result ralph.BeadResult }
	loopEndMsg         struct{ Result *ralph.CoreResult }
	loopErrorMsg       struct{ Err error }
	durationTickMsg    struct{}
	iterationStartMsg  struct{ Iteration int }
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

// Ensure Observer implements ProgressObserver.
var _ ralph.ProgressObserver = (*Observer)(nil)

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

// OnIterationStart is called when an iteration begins.
func (o *Observer) OnIterationStart(iteration int) {
	if o.program != nil {
		o.program.Send(iterationStartMsg{Iteration: iteration})
	}
}

// NewModel creates a new TUI model for the given Core.
func NewModel(core *ralph.Core) *Model {
	maxIter := core.MaxIterations
	if maxIter <= 0 {
		maxIter = ralph.DefaultMaxIterations
	}
	return &Model{
		core:       core,
		styles:     DefaultStyles(),
		maxIter:    maxIter,
		toolEvents: make([]ToolEventDisplay, 0, MaxToolEvents+1),
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

	case loopStartedMsg:
		m.mu.Lock()
		m.loopStarted = true
		m.beadID = msg.RootBead
		m.status = "Starting..."
		m.mu.Unlock()

	case beadStartMsg:
		m.mu.Lock()
		m.beadID = msg.Bead.ID
		m.beadTitle = msg.Bead.Title
		m.status = "Working"
		m.mu.Unlock()

	case iterationStartMsg:
		m.mu.Lock()
		m.iteration = msg.Iteration + 1 // Convert to 1-based
		m.toolEvents = m.toolEvents[:0]  // Clear tool events for new iteration
		m.mu.Unlock()

	case toolEventMsg:
		m.mu.Lock()
		if msg.Started {
			// Add new tool event
			event := ToolEventDisplay{
				Name:    msg.Event.Name,
				Detail:  extractToolDetail(msg.Event.Name, msg.Event.Attributes),
				Started: msg.Event.Timestamp,
			}
			m.toolEvents = append(m.toolEvents, event)
			// Keep only the last MaxToolEvents
			if len(m.toolEvents) > MaxToolEvents {
				m.toolEvents = m.toolEvents[len(m.toolEvents)-MaxToolEvents:]
			}
		} else {
			// Mark tool as completed by finding it in the list
			for i := len(m.toolEvents) - 1; i >= 0; i-- {
				if m.toolEvents[i].Name == msg.Event.Name && !m.toolEvents[i].Completed {
					m.toolEvents[i].Completed = true
					m.toolEvents[i].Duration = time.Since(m.toolEvents[i].Started)
					break
				}
			}
		}
		m.mu.Unlock()

	case beadCompleteMsg:
		m.mu.Lock()
		m.outcome = msg.Result.Outcome
		m.mu.Unlock()

	case loopEndMsg:
		m.mu.Lock()
		m.loopDone = true
		if msg.Result != nil {
			m.duration = msg.Result.Duration
			m.outcome = msg.Result.Outcome
			m.iteration = msg.Result.Iterations
		}
		m.status = "Complete"
		m.mu.Unlock()

	case loopErrorMsg:
		m.err = msg.Err
		return m, tea.Quit

	case durationTickMsg:
		if m.loopStarted && !m.loopDone {
			m.mu.Lock()
			m.duration = time.Since(m.loopStart)
			m.mu.Unlock()
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

	// Header
	header := m.styles.Title.Render("⚡ RALPH")
	if m.beadID != "" {
		header += " " + m.styles.Subtitle.Render("→ "+m.beadID)
	}
	b.WriteString(header)
	b.WriteString("\n\n")

	// Bead info
	m.mu.Lock()
	beadTitle := m.beadTitle
	iteration := m.iteration
	maxIter := m.maxIter
	toolEvents := make([]ToolEventDisplay, len(m.toolEvents))
	copy(toolEvents, m.toolEvents)
	loopStarted := m.loopStarted
	loopDone := m.loopDone
	duration := m.duration
	outcome := m.outcome
	m.mu.Unlock()

	if beadTitle != "" {
		b.WriteString(m.styles.Status.Render(beadTitle))
		b.WriteString("\n\n")
	}

	// Iteration progress
	if iteration > 0 {
		iterText := fmt.Sprintf("Iteration %d/%d", iteration, maxIter)
		b.WriteString(m.styles.Muted.Render(iterText))
		b.WriteString("\n")
	}

	// Recent tool events
	if len(toolEvents) > 0 {
		for _, event := range toolEvents {
			var icon, durStr string
			var style lipgloss.Style
			if event.Completed {
				icon = "✓"
				style = m.styles.Success
				durStr = " " + m.styles.Duration.Render(ralph.FormatDuration(event.Duration))
			} else {
				icon = "⚙"
				style = m.styles.ToolName
			}
			line := style.Render(icon+" "+event.Name)
			if event.Detail != "" {
				line += " " + m.styles.Muted.Render(event.Detail)
			}
			line += durStr
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")

	// Status bar
	var parts []string

	// Status indicator
	if loopDone {
		switch outcome {
		case ralph.OutcomeSuccess:
			parts = append(parts, m.styles.Success.Render("✓ Success"))
		case ralph.OutcomeQuestion:
			parts = append(parts, m.styles.Warning.Render("? Question"))
		case ralph.OutcomeTimeout:
			parts = append(parts, m.styles.Error.Render("⏱ Timeout"))
		case ralph.OutcomeMaxIterations:
			parts = append(parts, m.styles.Warning.Render("⚠ Max iterations"))
		default:
			parts = append(parts, m.styles.Error.Render("✗ Failed"))
		}
	} else if loopStarted {
		parts = append(parts, m.styles.Status.Render("● Running"))
	} else {
		parts = append(parts, m.styles.Muted.Render("○ Waiting"))
	}

	// Duration
	if loopStarted && duration > 0 {
		parts = append(parts, m.styles.Muted.Render(ralph.FormatDuration(duration)))
	}

	b.WriteString(strings.Join(parts, " │ "))

	// Quit hint
	if loopDone {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Muted.Render("Press q to quit"))
	}

	return b.String()
}

// extractToolDetail extracts a display-friendly detail from tool attributes.
// Uses shortenPath from trace_view.go and textutil.Truncate for unicode-aware truncation.
func extractToolDetail(toolName string, attrs map[string]string) string {
	switch toolName {
	case "Read", "read":
		if path, ok := attrs["file_path"]; ok {
			return shortenPath(path)
		}
		if path, ok := attrs["path"]; ok {
			return shortenPath(path)
		}
	case "Write", "write", "StrReplace", "edit":
		if path, ok := attrs["file_path"]; ok {
			return shortenPath(path)
		}
		if path, ok := attrs["path"]; ok {
			return shortenPath(path)
		}
	case "Shell", "shell", "Bash":
		if cmd, ok := attrs["command"]; ok {
			// Normalize whitespace and truncate using visual width
			cmd = strings.ReplaceAll(cmd, "\n", " ")
			cmd = strings.ReplaceAll(cmd, "\t", " ")
			for strings.Contains(cmd, "  ") {
				cmd = strings.ReplaceAll(cmd, "  ", " ")
			}
			cmd = strings.TrimSpace(cmd)
			return textutil.Truncate(cmd, 50)
		}
	case "Grep", "grep", "SemanticSearch", "search":
		if q, ok := attrs["query"]; ok {
			return textutil.Truncate(q, 40)
		}
		if p, ok := attrs["pattern"]; ok {
			return textutil.Truncate(p, 40)
		}
	case "Glob", "glob":
		if p, ok := attrs["glob_pattern"]; ok {
			return p
		}
		if p, ok := attrs["pattern"]; ok {
			return p
		}
	}
	return ""
}
