package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestKeybindRegistryLeaderHints(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	reg.BindWithDesc("SPC s s", tea.Quit, "Open shell")
	reg.BindWithDesc("SPC s a", tea.Quit, "Launch agent")

	hints := reg.LeaderHints("")
	require.Equal(t, "Quit", hints["q"])
	require.Equal(t, "Shell", hints["s"])

	subHints := reg.LeaderHints("SPC s")
	require.Equal(t, "Open shell", subHints["s"])
	require.Equal(t, "Launch agent", subHints["a"])
}

func TestKeyHandlerLeaderFlow(t *testing.T) {
	reg := NewKeybindRegistry()
	var called bool
	reg.BindWithDesc("SPC x", func() tea.Msg {
		called = true
		return nil
	}, "Do x")

	h := NewKeyHandler(reg)
	consumed, cmd := h.Handle(keyMsg(" "))
	require.True(t, consumed)
	require.Nil(t, cmd)
	require.True(t, h.LeaderWaiting)

	consumed, cmd = h.Handle(keyMsg("x"))
	require.True(t, consumed)
	require.NotNil(t, cmd)
	cmd()
	require.True(t, called)
	require.False(t, h.LeaderWaiting)
}

func TestKeyHandlerEscCancelsLeader(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.BindWithDesc("SPC x", tea.Quit, "Do x")
	h := NewKeyHandler(reg)

	consumed, cmd := h.Handle(keyMsg(" "))
	require.True(t, consumed)
	require.Nil(t, cmd)
	require.True(t, h.LeaderWaiting)

	consumed, cmd = h.Handle(keyMsg("esc"))
	require.True(t, consumed)
	require.Nil(t, cmd)
	require.False(t, h.LeaderWaiting)
	require.Empty(t, h.Buffer)
}

func TestKeyHandlerMultiKeyPrefix(t *testing.T) {
	reg := NewKeybindRegistry()
	var called bool
	reg.BindWithDesc("SPC s s", func() tea.Msg {
		called = true
		return nil
	}, "Open shell")
	h := NewKeyHandler(reg)

	consumed, cmd := h.Handle(keyMsg(" "))
	require.True(t, consumed)
	require.Nil(t, cmd)

	consumed, cmd = h.Handle(keyMsg("s"))
	require.True(t, consumed)
	require.Nil(t, cmd)
	require.True(t, h.LeaderWaiting)

	consumed, cmd = h.Handle(keyMsg("s"))
	require.True(t, consumed)
	require.NotNil(t, cmd)
	cmd()
	require.True(t, called)
	require.False(t, h.LeaderWaiting)
}

func TestKeyHandlerSingleKeyOutsideLeader(t *testing.T) {
	reg := NewKeybindRegistry()
	var called bool
	reg.BindWithDesc("q", func() tea.Msg {
		called = true
		return nil
	}, "Quit")
	h := NewKeyHandler(reg)

	consumed, cmd := h.Handle(keyMsg("q"))
	require.True(t, consumed)
	require.NotNil(t, cmd)
	cmd()
	require.True(t, called)
}

func TestNewKeyMapShortHelpIncludesEsc(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	h := NewKeyHandler(reg)

	keyMap := NewKeyMap(reg, h)
	bindings := keyMap.ShortHelp()
	require.NotEmpty(t, bindings)

	hasEsc := false
	for _, b := range bindings {
		if len(b.Keys()) > 0 && b.Keys()[0] == "esc" {
			hasEsc = true
		}
	}
	require.True(t, hasEsc)
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

