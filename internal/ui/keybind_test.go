package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeybindRegistry_BindLookup(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.Bind("q", tea.Quit)
	reg.Bind("SPC q", tea.Quit)
	reg.Bind("j", nil)

	if reg.Lookup("q") == nil {
		t.Error("expected q to be bound")
	}
	if reg.Lookup("SPC q") == nil {
		t.Error("expected SPC q to be bound")
	}
	if reg.Lookup("unknown") != nil {
		t.Error("expected unknown to be unbound")
	}
}

func TestKeyHandler_LeaderKey(t *testing.T) {
	reg := NewKeybindRegistry()
	var executed bool
	reg.Bind("SPC x", func() tea.Msg {
		executed = true
		return nil
	})
	h := NewKeyHandler(reg)

	// Press space -> leader waiting (Bubble Tea reports space as " ")
	consumed, cmd := h.Handle(keyMsg(" "))
	if !consumed || cmd != nil {
		t.Errorf("space: consumed=%v cmd=%v", consumed, cmd)
	}
	if !h.LeaderWaiting {
		t.Error("expected leader waiting after space")
	}

	// Press x -> execute SPC x
	consumed, cmd = h.Handle(keyMsg("x"))
	if !consumed {
		t.Errorf("x: expected consumed")
	}
	if h.LeaderWaiting {
		t.Error("leader should not be waiting after completing sequence")
	}
	if cmd != nil {
		cmd()
		if !executed {
			t.Error("expected command to execute")
		}
	}
}

func TestKeyHandler_EscCancelsLeader(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.Bind("SPC x", tea.Quit)
	h := NewKeyHandler(reg)

	h.Handle(keyMsg(" "))
	if !h.LeaderWaiting {
		t.Fatal("expected leader waiting")
	}

	consumed, cmd := h.Handle(keyMsg("esc"))
	if !consumed || cmd != nil {
		t.Errorf("esc: consumed=%v cmd=%v", consumed, cmd)
	}
	if h.LeaderWaiting {
		t.Error("esc should cancel leader mode")
	}
}

func TestKeyHandler_SingleKey(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.Bind("q", tea.Quit)
	h := NewKeyHandler(reg)

	consumed, cmd := h.Handle(keyMsg("q"))
	if !consumed || cmd == nil {
		t.Errorf("q: consumed=%v cmd=%v", consumed, cmd)
	}
}

func TestKeyHandler_UnboundFallsThrough(t *testing.T) {
	reg := NewKeybindRegistry()
	reg.Bind("q", tea.Quit)
	h := NewKeyHandler(reg)

	consumed, _ := h.Handle(keyMsg("j"))
	if consumed {
		t.Error("unbound j should not be consumed")
	}
}

// keyMsg creates a tea.KeyMsg for testing. Bubble Tea uses KeyType and Runes.
// KeySpace.String() returns " ", KeyEsc returns "esc", etc.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "space", " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "q":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	case "x":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	case "j":
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
