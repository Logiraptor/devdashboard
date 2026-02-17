package tui

import (
	"fmt"
	"strings"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/ralph"

	"github.com/charmbracelet/lipgloss"
)

// MaxEvents is the number of recent events to display per agent
const MaxEvents = 4

// AgentEvent represents a single event in an agent's activity stream
type AgentEvent struct {
	Type      string    // "tool_start", "tool_end", "status", etc.
	Name      string    // Tool name or status message
	Detail    string    // Additional detail (file path, command, etc.)
	Timestamp time.Time
	Duration  time.Duration // For completed events
}

// AgentBlock tracks the state of a single agent working on a bead
type AgentBlock struct {
	BeadID    string
	BeadTitle string
	Status    string        // "running", "success", "failed", "timeout", "question"
	Events    []AgentEvent  // Ring buffer of last N events
	StartTime time.Time
	Duration  time.Duration
	IterNum   int // Which iteration number this is
}

// NewAgentBlock creates a new agent block for a bead
func NewAgentBlock(bead beads.Bead, iterNum int) *AgentBlock {
	return &AgentBlock{
		BeadID:    bead.ID,
		BeadTitle: bead.Title,
		Status:    "running",
		Events:    make([]AgentEvent, 0, MaxEvents+1),
		StartTime: time.Now(),
		IterNum:   iterNum,
	}
}

// AddEvent adds a new event to the block's event stream
func (b *AgentBlock) AddEvent(event AgentEvent) {
	b.Events = append(b.Events, event)
	// Keep only the last MaxEvents
	if len(b.Events) > MaxEvents {
		b.Events = b.Events[len(b.Events)-MaxEvents:]
	}
}

// AddToolStart adds a tool start event
func (b *AgentBlock) AddToolStart(name string, attrs map[string]string) {
	detail := extractToolDetail(name, attrs)
	b.AddEvent(AgentEvent{
		Type:      "tool_start",
		Name:      name,
		Detail:    detail,
		Timestamp: time.Now(),
	})
}

// AddToolEnd adds a tool end event (updates existing or adds new)
func (b *AgentBlock) AddToolEnd(name string, attrs map[string]string, duration time.Duration) {
	detail := extractToolDetail(name, attrs)
	b.AddEvent(AgentEvent{
		Type:      "tool_end",
		Name:      name,
		Detail:    detail,
		Timestamp: time.Now(),
		Duration:  duration,
	})
}

// SetStatus updates the agent's status
func (b *AgentBlock) SetStatus(status string) {
	b.Status = status
	if status != "running" {
		b.Duration = time.Since(b.StartTime)
	}
}

// extractToolDetail extracts the key detail from tool attributes
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
			return truncateCmd(cmd, 50)
		}
	case "Grep", "grep", "SemanticSearch", "search":
		if q, ok := attrs["query"]; ok {
			return truncate(q, 40)
		}
		if p, ok := attrs["pattern"]; ok {
			return truncate(p, 40)
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

// truncateCmd truncates a shell command, stripping cd prefixes and preferring to show the actual command
func truncateCmd(cmd string, max int) string {
	// Remove newlines for display
	cmd = strings.ReplaceAll(cmd, "\n", " ")
	cmd = strings.TrimSpace(cmd)

	// Strip "cd /path && " prefix - common when running in worktrees
	if strings.HasPrefix(cmd, "cd ") {
		if idx := strings.Index(cmd, " && "); idx != -1 {
			cmd = strings.TrimSpace(cmd[idx+4:])
		}
	}

	return truncate(cmd, max)
}

// AgentBlockStyles contains styles for rendering agent blocks
type AgentBlockStyles struct {
	// Block frame
	Border          lipgloss.Style
	BorderActive    lipgloss.Style
	BorderSuccess   lipgloss.Style
	BorderFailed    lipgloss.Style
	BorderQuestion  lipgloss.Style

	// Header line
	BeadID    lipgloss.Style
	BeadTitle lipgloss.Style
	Status    lipgloss.Style
	Duration  lipgloss.Style

	// Event lines
	EventIcon   lipgloss.Style
	ToolName    lipgloss.Style
	EventDetail lipgloss.Style
	EventTime   lipgloss.Style

	// Status indicators
	RunningIcon  lipgloss.Style
	SuccessIcon  lipgloss.Style
	FailedIcon   lipgloss.Style
	QuestionIcon lipgloss.Style
}

// DefaultAgentBlockStyles returns styles for agent blocks
func DefaultAgentBlockStyles() AgentBlockStyles {
	// Catppuccin-inspired palette
	lavender := lipgloss.Color("#b4befe")
	mauve := lipgloss.Color("#cba6f7")
	pink := lipgloss.Color("#f5c2e7")
	green := lipgloss.Color("#a6e3a1")
	red := lipgloss.Color("#f38ba8")
	yellow := lipgloss.Color("#f9e2af")
	blue := lipgloss.Color("#89b4fa")
	surface0 := lipgloss.Color("#313244")
	subtext0 := lipgloss.Color("#a6adc8")
	subtext1 := lipgloss.Color("#bac2de")

	return AgentBlockStyles{
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(surface0).
			Padding(0, 1),
		BorderActive: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(blue).
			Padding(0, 1),
		BorderSuccess: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(green).
			Padding(0, 1),
		BorderFailed: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(red).
			Padding(0, 1),
		BorderQuestion: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(yellow).
			Padding(0, 1),

		BeadID: lipgloss.NewStyle().
			Bold(true).
			Foreground(mauve),
		BeadTitle: lipgloss.NewStyle().
			Foreground(subtext1),
		Status: lipgloss.NewStyle().
			Foreground(subtext0),
		Duration: lipgloss.NewStyle().
			Foreground(subtext0),

		EventIcon: lipgloss.NewStyle().
			Foreground(lavender),
		ToolName: lipgloss.NewStyle().
			Foreground(pink),
		EventDetail: lipgloss.NewStyle().
			Foreground(subtext0),
		EventTime: lipgloss.NewStyle().
			Foreground(surface0),

		RunningIcon: lipgloss.NewStyle().
			Foreground(blue),
		SuccessIcon: lipgloss.NewStyle().
			Foreground(green),
		FailedIcon: lipgloss.NewStyle().
			Foreground(red),
		QuestionIcon: lipgloss.NewStyle().
			Foreground(yellow),
	}
}

// spinnerFrames for animated spinner
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerFrame returns the current spinner frame based on time
func SpinnerFrame() string {
	idx := int(time.Now().UnixMilli()/100) % len(spinnerFrames)
	return spinnerFrames[idx]
}

// Render renders an agent block as a styled string
func (b *AgentBlock) Render(styles AgentBlockStyles, width int) string {
	// Content width accounting for border padding
	contentWidth := width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	var lines []string

	// Line 1: Bead ID and title with status indicator
	headerIcon := SpinnerFrame() // Animated spinner for running
	headerIconStyle := styles.RunningIcon
	switch b.Status {
	case "success":
		headerIcon = "✓"
		headerIconStyle = styles.SuccessIcon
	case "failed", "timeout":
		headerIcon = "✗"
		headerIconStyle = styles.FailedIcon
	case "question":
		headerIcon = "?"
		headerIconStyle = styles.QuestionIcon
	}

	// Calculate available width for title
	idStr := styles.BeadID.Render(b.BeadID)
	iconStr := headerIconStyle.Render(headerIcon)
	idLen := lipgloss.Width(b.BeadID) + lipgloss.Width(headerIcon) + 3 // spaces
	titleMaxLen := contentWidth - idLen - 10                           // reserve for duration
	if titleMaxLen < 10 {
		titleMaxLen = 10
	}

	title := b.BeadTitle
	if len(title) > titleMaxLen {
		title = title[:titleMaxLen-1] + "…"
	}
	titleStr := styles.BeadTitle.Render(title)

	// Add duration for all agents
	durationStr := ""
	if b.Duration > 0 {
		durationStr = " " + styles.Duration.Render(ralph.FormatDuration(b.Duration))
	}

	header := fmt.Sprintf("%s %s %s%s", iconStr, idStr, titleStr, durationStr)
	lines = append(lines, header)

	// Lines 2-5: Recent events (or padding if fewer events)
	for i := 0; i < MaxEvents; i++ {
		if i < len(b.Events) {
			event := b.Events[i]
			eventLine := renderEvent(event, styles, contentWidth)
			lines = append(lines, eventLine)
		} else {
			// Dim placeholder line for consistent block height
			lines = append(lines, styles.EventDetail.Render("  ·"))
		}
	}

	content := strings.Join(lines, "\n")

	// Choose border style based on status
	borderStyle := styles.Border
	switch b.Status {
	case "running":
		borderStyle = styles.BorderActive
	case "success":
		borderStyle = styles.BorderSuccess
	case "failed", "timeout":
		borderStyle = styles.BorderFailed
	case "question":
		borderStyle = styles.BorderQuestion
	}

	return borderStyle.Width(contentWidth).Render(content)
}

// toolIcons maps tool names to aesthetic icons (Unicode symbols for terminal compatibility)
var toolIcons = map[string]string{
	"Read":           "◀",
	"read":           "◀",
	"Write":          "▶",
	"write":          "▶",
	"StrReplace":     "◆",
	"edit":           "◆",
	"Shell":          "⬢",
	"shell":          "⬢",
	"Bash":           "⬢",
	"Grep":           "◉",
	"grep":           "◉",
	"SemanticSearch": "◎",
	"search":         "◉",
	"Glob":           "◈",
	"glob":           "◈",
	"Delete":         "✕",
	"WebFetch":       "⬡",
	"TodoWrite":      "▣",
}

// getToolIcon returns an icon for a tool, with fallback
func getToolIcon(toolName string) string {
	if icon, ok := toolIcons[toolName]; ok {
		return icon
	}
	return "▸"
}

// renderEvent renders a single event line
func renderEvent(event AgentEvent, styles AgentBlockStyles, maxWidth int) string {
	// Get tool-specific icon
	icon := getToolIcon(event.Name)

	toolName := event.Name
	detail := event.Detail

	// Truncate detail to fit
	toolLen := len(toolName)
	availDetail := maxWidth - toolLen - 8 // icon + spaces + emoji width
	if availDetail < 10 {
		availDetail = 10
	}
	if len(detail) > availDetail {
		detail = detail[:availDetail-1] + "…"
	}

	line := fmt.Sprintf("  %s %s",
		icon,
		styles.ToolName.Render(toolName))

	if detail != "" {
		line += " " + styles.EventDetail.Render(detail)
	}

	// Add duration for completed tools
	if event.Duration > 0 {
		line += " " + styles.EventTime.Render(ralph.FormatDuration(event.Duration))
	}

	return line
}
