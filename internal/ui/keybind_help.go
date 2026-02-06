package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderKeybindHelp produces the transient help view shown after SPC.
// Displays SPC-prefixed bindings in a compact bar format.
// When keyHandler is in leader mode with a buffer (e.g. "SPC p"), shows next-level hints.
func RenderKeybindHelp(keyHandler *KeyHandler) string {
	if keyHandler == nil {
		return ""
	}
	currentSeq := ""
	if len(keyHandler.Buffer) > 0 {
		currentSeq = strings.Join(keyHandler.Buffer, " ")
	}
	hints := keyHandler.Registry.LeaderHints(currentSeq)
	if len(hints) == 0 {
		return ""
	}

	// Sort keys for stable display
	keys := make([]string, 0, len(hints))
	for k := range hints {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(0, 1).
		MarginTop(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Bold(false)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	var parts []string
	for _, k := range keys {
		desc := hints[k]
		parts = append(parts, keyStyle.Render(k)+": "+descStyle.Render(desc))
	}

	prefix := "SPC"
	if currentSeq != "" {
		prefix = currentSeq
	}
	content := labelStyle.Render(prefix) + " " + strings.Join(parts, "  ")
	content += "  " + descStyle.Render("[esc] cancel")
	return boxStyle.Render(content)
}
