package ralph

import "github.com/charmbracelet/lipgloss"

// Color constants
const (
	ColorPrimary   = "39"  // Blue
	ColorSuccess   = "42"  // Green
	ColorWarning   = "214" // Orange
	ColorError     = "196" // Red
	ColorMuted     = "245" // Gray
	ColorHighlight = "212" // Pink
)

// RalphStyles contains all styles for ralph TUI
type RalphStyles struct {
	Title      lipgloss.Style
	Subtitle   lipgloss.Style
	Status     lipgloss.Style
	Success    lipgloss.Style
	Error      lipgloss.Style
	Warning    lipgloss.Style
	Muted      lipgloss.Style
	TreeBranch lipgloss.Style
	BeadID     lipgloss.Style
	Duration   lipgloss.Style
	ToolName   lipgloss.Style
	Border     lipgloss.Style
}

// DefaultStyles returns the default ralph styles
func DefaultStyles() RalphStyles {
	return RalphStyles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ColorPrimary)),
		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorHighlight)),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)),
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorSuccess)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorError)),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorWarning)),
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)),
		TreeBranch: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)),
		BeadID: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ColorPrimary)),
		Duration: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)),
		ToolName: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorHighlight)),
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorMuted)),
	}
}

// Status icons
const (
	IconRunning  = "●"
	IconSuccess  = "✓"
	IconFailed   = "✗"
	IconTimeout  = "⏱"
	IconQuestion = "?"
	IconSkipped  = "⊘"
)

// StatusIcon returns the appropriate icon for an outcome
func StatusIcon(outcome Outcome) string {
	switch outcome {
	case OutcomeSuccess:
		return IconSuccess
	case OutcomeFailure:
		return IconFailed
	case OutcomeTimeout:
		return IconTimeout
	case OutcomeQuestion:
		return IconQuestion
	default:
		return IconRunning
	}
}

// StatusStyle returns the appropriate style for an outcome
func (s RalphStyles) StatusStyle(outcome Outcome) lipgloss.Style {
	switch outcome {
	case OutcomeSuccess:
		return s.Success
	case OutcomeFailure:
		return s.Error
	case OutcomeTimeout:
		return s.Warning
	case OutcomeQuestion:
		return s.Warning
	default:
		return s.Status
	}
}
