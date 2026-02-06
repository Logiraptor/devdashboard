package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderKeybindHelp produces the transient help view shown after SPC.
// Displays SPC-prefixed bindings in a compact bar format.
func RenderKeybindHelp(reg *KeybindRegistry) string {
	hints := reg.LeaderHints()
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

	content := labelStyle.Render("SPC") + " " + strings.Join(parts, "  ")
	content += "  " + descStyle.Render("[esc] cancel")
	return boxStyle.Render(content)
}
