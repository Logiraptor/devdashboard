package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// Theme colors used throughout the UI
const (
	ColorAccent   = "86"   // Cyan/green - for titles, highlights
	ColorHighlight = "205" // Magenta - for selected items, borders
	ColorDanger   = "196"  // Red - for warnings, errors
	ColorMuted    = "241"  // Gray - for dimmed text, hints
	ColorText     = "252"  // Light gray - for normal text
	ColorDim      = "243"  // Darker gray - for very dim text
	ColorWarning  = "208"  // Orange - for warning details
)

// Styles contains shared style definitions used across views and modals.
var Styles = struct {
	// Title styles
	Title      lipgloss.Style // Bold accent color - for main titles
	TitleWarning lipgloss.Style // Bold danger color - for warning titles

	// Box styles
	Box        lipgloss.Style // Standard box with rounded border (accent border)
	BoxDanger  lipgloss.Style // Warning/error box (danger border)
	BoxCompact lipgloss.Style // Compact box with less padding (for lists)

	// Text styles
	Selected   lipgloss.Style // Highlighted/selected items (bold highlight color)
	Muted      lipgloss.Style // Dimmed text (muted color)
	Normal     lipgloss.Style // Normal text (text color)
	Hint       lipgloss.Style // Help/hint text (muted color)
	Status     lipgloss.Style // Status indicators (accent color)
	Section    lipgloss.Style // Section headers (highlight color)
	Empty      lipgloss.Style // Empty state text (muted, italic)
	Label      lipgloss.Style // Modal label/content (default)
	Details    lipgloss.Style // Warning details (warning color)
}{
	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorAccent)),
	TitleWarning: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorDanger)),
	Box: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorHighlight)).
		Padding(1, 2).
		Margin(1),
	BoxDanger: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorDanger)).
		Padding(1, 2).
		Margin(1),
	BoxCompact: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorHighlight)).
		Padding(0, 1).
		Margin(1),
	Selected: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorHighlight)).
		Bold(true),
	Muted: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorMuted)),
	Normal: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorText)),
	Hint: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorMuted)),
	Status: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)),
	Section: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorHighlight)),
	Empty: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorMuted)).
		Italic(true),
	Label: lipgloss.NewStyle(),
	Details: lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorWarning)),
}

// NewCompactListDelegate returns a delegate with zero spacing and shared styles.
// This factory standardizes list delegate configuration across the codebase.
func NewCompactListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	d.ShowDescription = false
	d.Styles.SelectedTitle = Styles.Selected
	d.Styles.SelectedDesc = Styles.Selected
	d.Styles.NormalTitle = Styles.Muted
	d.Styles.NormalDesc = Styles.Muted
	return d
}

// ModalStyles is kept for backward compatibility but now delegates to Styles.
// New code should use Styles directly.
var ModalStyles = struct {
	BoxDefault  lipgloss.Style
	BoxWarning  lipgloss.Style
	BoxCompact  lipgloss.Style
	Title       lipgloss.Style
	TitleWarning lipgloss.Style
	Label       lipgloss.Style
	Help        lipgloss.Style
	Details     lipgloss.Style
}{
	BoxDefault:  Styles.Box,
	BoxWarning:  Styles.BoxDanger,
	BoxCompact:  Styles.BoxCompact,
	Title:       Styles.Title,
	TitleWarning: Styles.TitleWarning,
	Label:       Styles.Label,
	Help:        Styles.Hint,
	Details:     Styles.Details,
}
