// Package tui provides a terminal user interface for the ralph agent loop.
package tui

import (
	"devdeploy/internal/ralph"

	"github.com/charmbracelet/lipgloss"
)

// Color constants
const (
	ColorPrimary   = "39"  // Blue
	ColorSuccess   = "42"  // Green
	ColorWarning   = "214" // Orange
	ColorError     = "196" // Red
	ColorMuted     = "245" // Gray
	ColorHighlight = "212" // Pink
)

// Styles contains all styles for ralph TUI
type Styles struct {
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
func DefaultStyles() Styles {
	return Styles{
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
func StatusIcon(outcome ralph.Outcome) string {
	switch outcome {
	case ralph.OutcomeSuccess:
		return IconSuccess
	case ralph.OutcomeFailure:
		return IconFailed
	case ralph.OutcomeTimeout:
		return IconTimeout
	case ralph.OutcomeQuestion:
		return IconQuestion
	default:
		return IconRunning
	}
}

// StatusStyle returns the appropriate style for an outcome
func (s Styles) StatusStyle(outcome ralph.Outcome) lipgloss.Style {
	switch outcome {
	case ralph.OutcomeSuccess:
		return s.Success
	case ralph.OutcomeFailure:
		return s.Error
	case ralph.OutcomeTimeout:
		return s.Warning
	case ralph.OutcomeQuestion:
		return s.Warning
	default:
		return s.Status
	}
}
