// Package tui provides a terminal user interface for the ralph agent loop.
package tui

import (
	"devdeploy/internal/ralph"
	"devdeploy/internal/ui"

	"github.com/charmbracelet/lipgloss"
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
// Uses unified color palette from ui package
func DefaultStyles() Styles {
	return Styles{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ui.ColorAccent)),
		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorHighlight)),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorMuted)),
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorSuccess)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorDanger)),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorWarning)),
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorMuted)),
		TreeBranch: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorMuted)),
		BeadID: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(ui.ColorAccent)),
		Duration: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorMuted)),
		ToolName: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ui.ColorHighlight)),
		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ui.ColorMuted)),
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
