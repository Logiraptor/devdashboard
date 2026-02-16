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
	core         *ralph.Core
	traceView    *TraceViewModel
	traceEmitter *LocalTraceEmitter
	styles       Styles
	summary      summary
	status       string
	err          error
	loopStarted  bool
	loopDone     bool
	width        int
	height       int
	program      *tea.Program
	ctx          context.Context
	cancel       context.CancelFunc
	loopStart    time.Time
	iterNum      int // Current iteration number

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

// Observer implements ralph.ProgressObserver and forwards events to the TUI.
type Observer struct {
	ralph.NoopObserver
	program      *tea.Program
	traceEmitter *LocalTraceEmitter
	toolSpans    map[string]string // tool call ID → span ID
	mu           sync.Mutex
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
	if o.traceEmitter == nil {
		return
	}
	attrs := event.Attributes // already a map[string]string
	spanID := o.traceEmitter.StartTool(event.Name, attrs)

	// Track span ID for matching end event
	o.mu.Lock()
	if o.toolSpans == nil {
		o.toolSpans = make(map[string]string)
	}
	// Use a key that uniquely identifies this tool call
	o.toolSpans[event.ID] = spanID
	o.mu.Unlock()
}

// OnToolEnd is called when a tool call ends.
func (o *Observer) OnToolEnd(event ralph.ToolEvent) {
	if o.traceEmitter == nil {
		return
	}
	o.mu.Lock()
	spanID := o.toolSpans[event.ID]
	delete(o.toolSpans, event.ID)
	o.mu.Unlock()

	if spanID != "" {
		o.traceEmitter.EndTool(spanID, event.Attributes)
	}
}

// NewModel creates a new TUI model for the given Core.
func NewModel(core *ralph.Core) *Model {
	styles := DefaultStyles()
	return &Model{
		core:         core,
		traceView:    NewTraceViewModel(styles),
		traceEmitter: NewLocalTraceEmitter(),
		styles:       styles,
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
	m.traceEmitter.SetProgram(p)

	// Set up observer to forward events to TUI
	tuiObserver := &Observer{
		program:      p,
		traceEmitter: m.traceEmitter,
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

		// Start duration ticker
		ticker := time.NewTicker(time.Second)
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
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
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
		// Reserve space for header/footer
		traceHeight := msg.Height - 4
		if traceHeight < 5 {
			traceHeight = 5
		}
		m.traceView.SetSize(msg.Width, traceHeight)

	case TraceUpdateMsg:
		m.traceView.SetTrace(msg.Trace)

	case loopStartedMsg:
		m.mu.Lock()
		m.loopStarted = true
		m.status = fmt.Sprintf("Loop started: %s", msg.RootBead)
		m.mu.Unlock()

	case beadStartMsg:
		m.mu.Lock()
		m.iterNum++
		m.status = fmt.Sprintf("Working on %s: %s", msg.Bead.ID, msg.Bead.Title)
		m.mu.Unlock()

		// Also update trace emitter
		m.traceEmitter.StartIteration(msg.Bead.ID, msg.Bead.Title, m.iterNum)

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
		m.mu.Unlock()

		// Update trace emitter
		attrs := map[string]string{}
		if r.ChatID != "" {
			attrs["chat_id"] = r.ChatID
		}
		if r.ExitCode != 0 {
			attrs["exit_code"] = fmt.Sprintf("%d", r.ExitCode)
		}
		// Note: We need the spanID from StartIteration, but we don't have it here.
		// For now, trace emitter handles this internally.

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
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m *Model) View() string {
	if m.err != nil {
		return m.styles.Error.Render(fmt.Sprintf("Error: %v\n", m.err))
	}

	var b strings.Builder

	// Header
	header := m.styles.Title.Render("Ralph Loop")
	if m.core.RootBead != "" {
		header += " " + m.styles.Subtitle.Render(m.core.RootBead)
	}
	b.WriteString(header)
	b.WriteString("\n")

	// Trace view
	b.WriteString(m.traceView.View())
	b.WriteString("\n")

	// Status bar
	m.mu.Lock()
	statusParts := make([]string, 0, 5)

	if m.loopDone {
		statusParts = append(statusParts, m.styles.Success.Render("✓ Completed"))
	} else if m.loopStarted {
		statusParts = append(statusParts, m.styles.Status.Render("● Running"))
	} else {
		statusParts = append(statusParts, m.styles.Muted.Render("Waiting..."))
	}

	if m.status != "" {
		statusParts = append(statusParts, m.styles.Status.Render(m.status))
	}

	if m.summary.Iterations > 0 {
		stats := fmt.Sprintf("%d done, %d failed", m.summary.Succeeded, m.summary.Failed)
		statusParts = append(statusParts, m.styles.Muted.Render(stats))
	}

	if m.loopStarted && m.summary.Duration > 0 {
		durationStr := ralph.FormatDuration(m.summary.Duration)
		statusParts = append(statusParts, m.styles.Muted.Render(durationStr))
	}

	lastFailure := m.lastFailure
	loopDone := m.loopDone
	m.mu.Unlock()

	statusLine := strings.Join(statusParts, " | ")
	b.WriteString(statusLine)

	// Show failure details if there was a failure
	if lastFailure != nil && (lastFailure.Outcome == ralph.OutcomeFailure || lastFailure.Outcome == ralph.OutcomeTimeout) {
		b.WriteString("\n\n")
		b.WriteString(m.styles.Error.Render("Last Failure:"))
		b.WriteString("\n")

		b.WriteString(fmt.Sprintf("  Bead: %s", lastFailure.Bead.ID))
		if lastFailure.ExitCode != 0 {
			b.WriteString(fmt.Sprintf(" (exit code %d)", lastFailure.ExitCode))
		}
		b.WriteString("\n")

		if lastFailure.ChatID != "" {
			b.WriteString(fmt.Sprintf("  ChatID: %s\n", m.styles.Muted.Render(lastFailure.ChatID)))
		}

		if lastFailure.ErrorMessage != "" {
			errMsg := lastFailure.ErrorMessage
			if len(errMsg) > 100 {
				errMsg = errMsg[:97] + "..."
			}
			b.WriteString(fmt.Sprintf("  Error: %s\n", m.styles.Error.Render(errMsg)))
		} else if lastFailure.Stderr != "" {
			stderrLines := strings.Split(strings.TrimSpace(lastFailure.Stderr), "\n")
			start := len(stderrLines) - 3
			if start < 0 {
				start = 0
			}
			b.WriteString("  Stderr:\n")
			for _, line := range stderrLines[start:] {
				if line = strings.TrimSpace(line); line != "" {
					if len(line) > 100 {
						line = line[:97] + "..."
					}
					b.WriteString(fmt.Sprintf("    %s\n", m.styles.Error.Render(line)))
				}
			}
		} else {
			b.WriteString(fmt.Sprintf("  %s\n", m.styles.Muted.Render("No error details available")))
		}
	}

	// Quit hint
	if loopDone {
		b.WriteString("\n")
		b.WriteString(m.styles.Muted.Render("Press q to quit"))
	}

	return b.String()
}
