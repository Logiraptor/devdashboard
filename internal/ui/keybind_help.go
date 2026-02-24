package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/lipgloss"
)

// RenderKeybindHelp produces the transient help view shown after SPC.
// Displays SPC-prefixed bindings in a compact bar format.
// When keyHandler is in leader mode with a buffer (e.g. "SPC p"), shows next-level hints.
// Uses bubbles/help.Model with a KeyMap for standard help rendering.
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

	keyMap := NewKeyMap(keyHandler.Registry, keyHandler)
	helpModel := help.New()
	helpModel.Styles.ShortKey = Styles.Selected
	helpModel.Styles.ShortDesc = Styles.Muted
	helpModel.Styles.ShortSeparator = Styles.Muted

	helpContent := helpModel.View(keyMap)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorAccent)).
		Padding(0, 1).
		MarginTop(1)

	labelStyle := Styles.Muted

	prefix := "SPC"
	if currentSeq != "" {
		prefix = currentSeq
	}
	content := labelStyle.Render(prefix) + " " + helpContent
	return boxStyle.Render(content)
}
