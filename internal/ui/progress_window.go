package ui

import (
	"fmt"
	"strings"

	"devdeploy/internal/progress"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ProgressWindow displays live agent output with scrollback.
// Shown as overlay when agent runs; Esc dismisses.
type ProgressWindow struct {
	events   []progress.Event
	viewport viewport.Model
	width    int
	height   int
}

// Ensure ProgressWindow implements View.
var _ View = (*ProgressWindow)(nil)

const defaultProgressWidth = 70
const defaultProgressHeight = 18

// NewProgressWindow creates an empty progress window.
func NewProgressWindow() *ProgressWindow {
	vp := viewport.New(defaultProgressWidth, defaultProgressHeight)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(0, 1)
	return &ProgressWindow{
		events:   nil,
		viewport: vp,
		width:    defaultProgressWidth,
		height:   defaultProgressHeight,
	}
}

// Init implements View.
func (p *ProgressWindow) Init() tea.Cmd {
	return p.viewport.Init()
}

// Update implements View.
func (p *ProgressWindow) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.Event:
		p.events = append(p.events, msg)
		p.refreshContent()
		p.viewport.GotoBottom()
		return p, nil
	case tea.KeyMsg:
		if msg.String() == "esc" {
			return p, func() tea.Msg { return DismissModalMsg{} }
		}
	case tea.WindowSizeMsg:
		p.width = msg.Width
		p.height = msg.Height
		// Use a portion of the window for the progress overlay
		w := msg.Width - 4
		h := msg.Height/2 + 4
		if w < 40 {
			w = 40
		}
		if h < 12 {
			h = 12
		}
		p.viewport.Width = w
		p.viewport.Height = h
		p.refreshContent()
		return p, nil
	}

	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

// View implements View.
func (p *ProgressWindow) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	header := titleStyle.Render("Agent progress") + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  Esc: close")
	return header + "\n" + p.viewport.View()
}

// refreshContent rebuilds the viewport content from accumulated events.
func (p *ProgressWindow) refreshContent() {
	var lines []string
	for i, ev := range p.events {
		ts := ev.Timestamp.Format("15:04:05")
		statusIcon := statusIcon(ev.Status)
		line := fmt.Sprintf("[%s] %s %s", ts, statusIcon, ev.Message)
		lines = append(lines, line)
		if len(ev.Metadata) > 0 {
			for k, v := range ev.Metadata {
				lines = append(lines, fmt.Sprintf("      %s: %s", k, v))
			}
		}
		// Avoid trailing newline on last item
		if i < len(p.events)-1 {
			lines = append(lines, "")
		}
	}
	content := strings.Join(lines, "\n")
	if content == "" {
		content = "Waiting for agent output..."
	}
	p.viewport.SetContent(content)
	p.viewport.GotoBottom()
}

func statusIcon(s progress.Status) string {
	switch s {
	case progress.StatusRunning:
		return "●"
	case progress.StatusDone:
		return "✓"
	case progress.StatusError:
		return "✗"
	default:
		return "•"
	}
}
