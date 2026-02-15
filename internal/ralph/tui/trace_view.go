package tui

import (
	"fmt"
	"strings"

	"devdeploy/internal/ralph"
	"devdeploy/internal/trace"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// TraceViewModel displays the current trace as an ASCII tree
type TraceViewModel struct {
	trace    *trace.Trace
	viewport viewport.Model
	styles   Styles
	width    int
	height   int
}

// NewTraceViewModel creates a new trace view
func NewTraceViewModel(styles Styles) *TraceViewModel {
	vp := viewport.New(80, 20)
	return &TraceViewModel{
		viewport: vp,
		styles:   styles,
		width:    80,
		height:   20,
	}
}

// SetTrace updates the trace being displayed
func (v *TraceViewModel) SetTrace(t *trace.Trace) {
	v.trace = t
	v.refreshContent()
}

// SetSize sets the dimensions
func (v *TraceViewModel) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.Width = width
	v.viewport.Height = height
	v.refreshContent()
}

// Update handles messages (scrolling)
func (v *TraceViewModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return cmd
}

// View returns the rendered trace tree
func (v *TraceViewModel) View() string {
	return v.viewport.View()
}

// refreshContent rebuilds the viewport content
func (v *TraceViewModel) refreshContent() {
	if v.trace == nil {
		v.viewport.SetContent(v.styles.Muted.Render("No active trace"))
		return
	}

	var lines []string

	// Header
	var headerOutcome ralph.Outcome
	if v.trace.Status == "completed" {
		headerOutcome = ralph.OutcomeSuccess
	} else {
		headerOutcome = ralph.Outcome(-1) // Running
	}
	statusIcon := StatusIcon(headerOutcome)
	header := fmt.Sprintf("Trace: %s %s %s",
		shortID(v.trace.ID),
		v.styles.StatusStyle(headerOutcome).Render(statusIcon),
		v.styles.Muted.Render(v.trace.Status))
	lines = append(lines, v.styles.Title.Render(header))
	lines = append(lines, "")

	// Render span tree
	if v.trace.RootSpan != nil {
		for i, child := range v.trace.RootSpan.Children {
			isLast := i == len(v.trace.RootSpan.Children)-1
			childLines := v.renderIteration(child, "", isLast, i+1, len(v.trace.RootSpan.Children))
			lines = append(lines, childLines...)
		}
	} else {
		lines = append(lines, v.styles.Muted.Render("  (no iterations yet)"))
	}

	v.viewport.SetContent(strings.Join(lines, "\n"))

	// Auto-scroll to bottom if at bottom
	if v.viewport.AtBottom() {
		v.viewport.GotoBottom()
	}
}

// renderIteration renders an iteration span with its tool children
func (v *TraceViewModel) renderIteration(span *trace.Span, prefix string, isLast bool, iterNum, total int) []string {
	var lines []string

	// Connector
	connector := "├─"
	if isLast {
		connector = "└─"
	}

	// Bead info from attributes
	beadID := span.Attributes["bead_id"]
	beadTitle := span.Attributes["bead_title"]
	if beadTitle == "" {
		beadTitle = span.Name
	}

	// Truncate title
	maxTitleLen := v.width - len(prefix) - 40
	if maxTitleLen < 20 {
		maxTitleLen = 20
	}
	if len(beadTitle) > maxTitleLen {
		beadTitle = beadTitle[:maxTitleLen-3] + "..."
	}

	// Determine status
	var outcome ralph.Outcome
	if outcomeStr, ok := span.Attributes["outcome"]; ok {
		outcome = parseOutcome(outcomeStr)
	} else if span.Duration == 0 {
		outcome = ralph.Outcome(-1) // Running (no outcome attribute yet)
	} else {
		outcome = ralph.OutcomeSuccess // Default for completed spans
	}

	icon := StatusIcon(outcome)
	iconStyle := v.styles.StatusStyle(outcome)

	// Format: [1/5] bead-id "title" ✓ 45s
	line := fmt.Sprintf("%s%s [%d/%d] %s \"%s\" %s",
		prefix,
		v.styles.TreeBranch.Render(connector),
		iterNum, total,
		v.styles.BeadID.Render(beadID),
		beadTitle,
		iconStyle.Render(icon))

	if span.Duration > 0 {
		line += " " + v.styles.Duration.Render(ralph.FormatDuration(span.Duration))
	}

	// Show exit code for failures
	if outcome == ralph.OutcomeFailure || outcome == ralph.OutcomeTimeout {
		if exitCode, ok := span.Attributes["exit_code"]; ok && exitCode != "" && exitCode != "0" {
			line += " " + v.styles.Error.Render(fmt.Sprintf("(exit %s)", exitCode))
		}
	}

	lines = append(lines, line)

	// Show chat ID for failed iterations (on a separate line)
	if outcome == ralph.OutcomeFailure || outcome == ralph.OutcomeTimeout {
		if chatID, ok := span.Attributes["chat_id"]; ok && chatID != "" {
			childPrefix := prefix
			if isLast {
				childPrefix += "   "
			} else {
				childPrefix += "│  "
			}
			chatLine := fmt.Sprintf("%s  %s %s",
				childPrefix,
				v.styles.Muted.Render("ChatID:"),
				v.styles.Muted.Render(chatID))
			lines = append(lines, chatLine)
		}
	}

	// Render tool children
	childPrefix := prefix
	if isLast {
		childPrefix += "   "
	} else {
		childPrefix += "│  "
	}

	for i, child := range span.Children {
		isLastChild := i == len(span.Children)-1
		toolLines := v.renderTool(child, childPrefix, isLastChild)
		lines = append(lines, toolLines...)
	}

	return lines
}

// renderTool renders a tool call span
func (v *TraceViewModel) renderTool(span *trace.Span, prefix string, isLast bool) []string {
	connector := "├─"
	if isLast {
		connector = "└─"
	}

	// Calculate available space for detail text
	prefixLen := len(prefix)
	overhead := 25 // connector + tool name + icon + duration + spaces
	maxDetail := v.width - prefixLen - overhead
	if maxDetail < 20 {
		maxDetail = 20
	}
	if maxDetail > 80 {
		maxDetail = 80
	}

	// Tool name and key attribute
	toolName := span.Name
	detail := ""
	switch toolName {
	case "read", "edit", "write":
		if path, ok := span.Attributes["file_path"]; ok {
			detail = shortenPath(path)
		}
	case "shell":
		if cmd, ok := span.Attributes["command"]; ok {
			detail = truncate(cmd, maxDetail)
		}
	case "search", "grep":
		if q, ok := span.Attributes["query"]; ok {
			detail = truncate(q, maxDetail)
		} else if p, ok := span.Attributes["pattern"]; ok {
			detail = truncate(p, maxDetail)
		}
	}

	// Status
	icon := IconSuccess
	iconStyle := v.styles.Success
	if span.Duration == 0 {
		icon = IconRunning
		iconStyle = v.styles.Status
	}

	line := fmt.Sprintf("%s%s %s",
		prefix,
		v.styles.TreeBranch.Render(connector),
		v.styles.ToolName.Render(toolName))

	if detail != "" {
		line += " " + v.styles.Muted.Render(detail)
	}
	line += " " + iconStyle.Render(icon)
	if span.Duration > 0 {
		line += " " + v.styles.Duration.Render(ralph.FormatDuration(span.Duration))
	}

	return []string{line}
}

// Helper functions
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func parseOutcome(s string) ralph.Outcome {
	switch s {
	case "success":
		return ralph.OutcomeSuccess
	case "failure":
		return ralph.OutcomeFailure
	case "timeout":
		return ralph.OutcomeTimeout
	case "question":
		return ralph.OutcomeQuestion
	default:
		return ralph.OutcomeSuccess
	}
}
