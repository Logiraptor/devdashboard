package ui

import (
	"fmt"
	"strings"
	"time"

	"devdeploy/internal/trace"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// TraceUpdateMsg is sent when trace state changes
type TraceUpdateMsg struct {
	Trace *trace.Trace
}

// TraceView displays the current trace as an ASCII tree
type TraceView struct {
	trace    *trace.Trace
	viewport viewport.Model
	width    int
	height   int
	visible  bool
}

// Ensure TraceView implements View
var _ View = (*TraceView)(nil)

// NewTraceView creates a new trace view
func NewTraceView() *TraceView {
	vp := viewport.New(50, 20)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorHighlight)).
		Padding(0, 1)
	return &TraceView{
		trace:    nil,
		viewport: vp,
		width:    50,
		height:   20,
		visible:  false,
	}
}

// Init implements View
func (v *TraceView) Init() tea.Cmd {
	return v.viewport.Init()
}

// Update implements View
func (v *TraceView) Update(msg tea.Msg) (View, tea.Cmd) {
	if !v.visible {
		return v, nil
	}

	switch msg := msg.(type) {
	case TraceUpdateMsg:
		v.trace = msg.Trace
		v.refreshContent()
		// Auto-scroll to bottom if already at bottom
		if v.viewport.AtBottom() {
			v.viewport.GotoBottom()
		}
		return v, nil
	case tea.WindowSizeMsg:
		// Window size is handled by SetSize
		return v, nil
	case tea.KeyMsg:
		// Handle viewport scrolling keys
		switch msg.String() {
		case "j", "down":
			v.viewport.LineDown(1)
			return v, nil
		case "k", "up":
			v.viewport.LineUp(1)
			return v, nil
		case "ctrl+d", "pgdown":
			v.viewport.PageDown()
			return v, nil
		case "ctrl+u", "pgup":
			v.viewport.PageUp()
			return v, nil
		case "g", "home":
			v.viewport.GotoTop()
			return v, nil
		case "G", "end":
			v.viewport.GotoBottom()
			return v, nil
		}
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

// View implements View
func (v *TraceView) View() string {
	if !v.visible {
		return ""
	}
	return v.viewport.View()
}

// SetSize sets the size of the trace view
func (v *TraceView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.Width = width
	v.viewport.Height = height
	v.refreshContent()
}

// SetVisible sets whether the trace view is visible
func (v *TraceView) SetVisible(visible bool) {
	v.visible = visible
	if visible {
		v.refreshContent()
	}
}

// IsVisible returns whether the trace view is visible
func (v *TraceView) IsVisible() bool {
	return v.visible
}

// refreshContent rebuilds the viewport content from the current trace
func (v *TraceView) refreshContent() {
	if v.trace == nil {
		v.viewport.SetContent("No active trace")
		return
	}

	var lines []string

	// Render trace tree
	if v.trace.RootSpan != nil {
		// Root span is the loop span, its children are iterations
		rootSpan := v.trace.RootSpan
		totalIters := len(rootSpan.Children)
		
		// Render root span header (loop)
		rootDurationStr := formatDuration(rootSpan.Duration)
		if rootDurationStr == "" {
			rootDurationStr = "running..."
		}
		rootStatusIcon := "✓"
		rootStatusColor := "2"
		if v.trace.Status == "running" {
			rootStatusIcon = "●"
			rootStatusColor = ColorWarning
		}
		rootLine := fmt.Sprintf("Trace: %s (%s) %s", 
			shortTraceID(v.trace.ID), 
			rootDurationStr,
			lipgloss.NewStyle().Foreground(lipgloss.Color(rootStatusColor)).Render(rootStatusIcon+" "+v.trace.Status))
		lines = append(lines, Styles.Title.Render(rootLine))
		lines = append(lines, "")
		
		// Render iterations with indices
		if totalIters > 0 {
			for i, iter := range rootSpan.Children {
				isLastIter := i == totalIters-1
				iterLines := v.renderSpanWithIndex(iter, "", isLastIter, i+1, totalIters)
				lines = append(lines, iterLines...)
			}
		} else {
			lines = append(lines, Styles.Muted.Render("  (no iterations yet)"))
		}
	} else {
		lines = append(lines, Styles.Muted.Render("  (no spans yet)"))
	}

	content := strings.Join(lines, "\n")
	v.viewport.SetContent(content)
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// renderSpan recursively renders a span and its children as a tree
// Returns the lines for this span and its children
func (v *TraceView) renderSpan(span *trace.Span, prefix string, isLast bool) []string {
	return v.renderSpanWithIndex(span, prefix, isLast, 0, 0)
}

// renderSpanWithIndex renders a span with iteration index information
func (v *TraceView) renderSpanWithIndex(span *trace.Span, prefix string, isLast bool, iterIndex, totalIters int) []string {
	var lines []string

	// Determine status icon and color
	statusIcon := "✓"
	statusColor := "2" // green for completed

	// Format span name and duration
	durationStr := formatDuration(span.Duration)

	// Build the line
	connector := "├─"
	if isLast {
		connector = "└─"
	}

	// Format span display name
	spanName := span.Name
	if spanName == "" {
		spanName = "(unnamed)"
	}

	// Check if this is an iteration span (has "iteration" in attributes or name pattern)
	isIteration := false
	if iterIndex > 0 && totalIters > 0 {
		isIteration = true
	}

	// Add iteration prefix if applicable
	displayName := spanName
	if isIteration {
		displayName = fmt.Sprintf("[%d/%d] %s", iterIndex, totalIters, spanName)
	}

	// Truncate if too long
	maxNameLen := v.width - len(prefix) - len(connector) - 30 // reserve space for status/duration
	if maxNameLen < 0 {
		maxNameLen = 20
	}
	if len(displayName) > maxNameLen {
		displayName = displayName[:maxNameLen-3] + "..."
	}

	line := prefix + connector + " " + displayName
	if durationStr != "" {
		line += " " + Styles.Muted.Render(durationStr)
	}
	line += " " + lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(statusIcon)

	lines = append(lines, line)

	// Render children (tools within iterations, or nested spans)
	if len(span.Children) > 0 {
		childPrefix := prefix
		if isLast {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}

		for i, child := range span.Children {
			isLastChild := i == len(span.Children)-1
			childLines := v.renderSpanWithIndex(child, childPrefix, isLastChild, 0, 0)
			lines = append(lines, childLines...)
		}
	}

	return lines
}


// shortTraceID returns a shortened version of the trace ID for display
func shortTraceID(id string) string {
	if len(id) > 16 {
		return id[:16]
	}
	return id
}
