package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// RenderKeybindHelp produces the transient help view shown after SPC.
// Displays SPC-prefixed bindings in a compact bar format, filtered by mode.
// When keyHandler is in leader mode with a buffer (e.g. "SPC p"), shows next-level hints.
func RenderKeybindHelp(keyHandler *KeyHandler, mode AppMode) string {
	if keyHandler == nil {
		return ""
	}
	currentSeq := ""
	if len(keyHandler.Buffer) > 0 {
		currentSeq = strings.Join(keyHandler.Buffer, " ")
	}
	hints := keyHandler.Registry.LeaderHints(currentSeq, mode)
	if len(hints) == 0 {
		return ""
	}

	// Sort keys for stable display
	keys := make([]string, 0, len(hints))
	for k := range hints {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Convert hints to key.Binding slice for bubbles/help
	bindings := make([]key.Binding, 0, len(keys))
	for _, k := range keys {
		desc := hints[k]
		bindings = append(bindings, key.NewBinding(
			key.WithKeys(k),
			key.WithHelp(k, desc),
		))
	}
	// Add esc cancel binding
	bindings = append(bindings, key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	))

	// Create help model with custom styling
	helpModel := help.New()
	helpModel.Styles.ShortKey = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)
	helpModel.Styles.ShortDesc = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	helpModel.Styles.ShortSeparator = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	// Render help view
	helpContent := helpModel.ShortHelpView(bindings)

	// Wrap in box with prefix label
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(0, 1).
		MarginTop(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Bold(false)

	prefix := "SPC"
	if currentSeq != "" {
		prefix = currentSeq
	}
	content := labelStyle.Render(prefix) + " " + helpContent
	return boxStyle.Render(content)
}
