package ui

import "github.com/charmbracelet/lipgloss"

// ModalStyles contains shared style definitions for modals.
var ModalStyles = struct {
	// Box styles
	BoxDefault lipgloss.Style // Standard modal box (cyan border)
	BoxWarning lipgloss.Style // Warning/error modal box (red border)
	BoxCompact lipgloss.Style // Compact box with less padding (for lists)

	// Text styles
	Title      lipgloss.Style // Modal title
	TitleWarning lipgloss.Style // Warning title (red)
	Label      lipgloss.Style // Modal label/content
	Help       lipgloss.Style // Help text (dim gray)
	Details    lipgloss.Style // Warning details (orange)
}{
	BoxDefault: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Margin(1),
	BoxWarning: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Margin(1),
	BoxCompact: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(0, 1).
		Margin(1),
	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")),
	TitleWarning: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196")),
	Label: lipgloss.NewStyle(),
	Help: lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")),
	Details: lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")),
}
