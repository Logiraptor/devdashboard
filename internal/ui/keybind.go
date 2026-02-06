package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// KeybindRegistry maps key sequences to commands.
// Key sequences use spacemacs-style notation: "SPC" for space, "SPC f" for SPC then f.
// Single keys: "j", "k", "esc", "ctrl+c", "enter".
type KeybindRegistry struct {
	bindings     map[string]tea.Cmd
	descriptions map[string]string
}

// NewKeybindRegistry creates an empty registry.
func NewKeybindRegistry() *KeybindRegistry {
	return &KeybindRegistry{
		bindings:     make(map[string]tea.Cmd),
		descriptions: make(map[string]string),
	}
}

// Bind registers a key sequence to a command.
// Overwrites any existing binding for the sequence.
// Use BindWithDesc for human-readable hints in the help view.
func (r *KeybindRegistry) Bind(seq string, cmd tea.Cmd) {
	r.BindWithDesc(seq, cmd, "")
}

// BindWithDesc registers a key sequence with a description for the help view.
func (r *KeybindRegistry) BindWithDesc(seq string, cmd tea.Cmd, desc string) {
	n := normalizeSeq(seq)
	r.bindings[n] = cmd
	if desc != "" {
		r.descriptions[n] = desc
	}
}

// Lookup returns the command for a key sequence, or nil if not bound.
func (r *KeybindRegistry) Lookup(seq string) tea.Cmd {
	return r.bindings[normalizeSeq(seq)]
}

// HasPrefix returns true if any binding starts with seq and a space (i.e. more keys follow).
func (r *KeybindRegistry) HasPrefix(seq string) bool {
	prefix := normalizeSeq(seq) + " "
	for k := range r.bindings {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// Hints returns all bound sequences with descriptions for display.
// Keys are normalized sequences; values are descriptions (or the sequence if none set).
func (r *KeybindRegistry) Hints() map[string]string {
	out := make(map[string]string)
	for seq, cmd := range r.bindings {
		if cmd != nil {
			if d, ok := r.descriptions[seq]; ok && d != "" {
				out[seq] = d
			} else {
				out[seq] = seq
			}
		}
	}
	return out
}

// LeaderHints returns hints for SPC-prefixed bindings.
// When currentSeq is empty, returns first-level hints (e.g. "q", "p", "a").
// When currentSeq is e.g. "SPC p", returns next-level hints (e.g. "c", "d", "a", "r").
func (r *KeybindRegistry) LeaderHints(currentSeq string) map[string]string {
	out := make(map[string]string)
	prefix := "SPC "
	if currentSeq != "" {
		prefix = normalizeSeq(currentSeq) + " "
	}
	for seq, cmd := range r.bindings {
		if cmd != nil && strings.HasPrefix(seq, prefix) {
			rest := strings.TrimPrefix(seq, prefix)
			// For display, take only the next key (e.g. "c" from "c" or "a a")
			parts := strings.Fields(rest)
			key := rest
			if len(parts) > 0 {
				key = parts[0]
			}
			if d, ok := r.descriptions[seq]; ok && d != "" {
				out[key] = d
			} else {
				out[key] = seq
			}
		}
	}
	return out
}

// normalizeSeq converts tea key strings to our canonical format.
// "space" -> "SPC", "ctrl+c" -> "ctrl+c", "j" -> "j".
func normalizeSeq(seq string) string {
	parts := strings.Fields(seq)
	for i, p := range parts {
		if p == "space" || p == " " {
			parts[i] = "SPC"
		}
	}
	return strings.Join(parts, " ")
}

// KeyHandler manages leader key state and dispatches to the registry.
type KeyHandler struct {
	Registry     *KeybindRegistry
	LeaderKey    string   // "space" (tea.KeyMsg.String() format)
	LeaderSeq    string   // "SPC" (our format)
	LeaderWaiting bool   // true when waiting for key after leader
	Buffer       []string // accumulated sequence in leader mode
}

// NewKeyHandler creates a handler with SPC as leader.
// Bubble Tea reports space as " " (KeySpace), not "space".
func NewKeyHandler(reg *KeybindRegistry) *KeyHandler {
	return &KeyHandler{
		Registry:      reg,
		LeaderKey:     " ", // tea.KeyMsg.String() returns " " for space
		LeaderSeq:     "SPC",
		LeaderWaiting: false,
		Buffer:        nil,
	}
}

// Handle processes a KeyMsg. Returns (consumed, cmd).
// If consumed is true, the key was handled by the keybind system and should not be passed to views.
// cmd is the command to run, if any.
func (h *KeyHandler) Handle(msg tea.KeyMsg) (consumed bool, cmd tea.Cmd) {
	s := msg.String()

	// Esc cancels leader mode
	if s == "esc" {
		if h.LeaderWaiting {
			h.LeaderWaiting = false
			h.Buffer = nil
			return true, nil
		}
		return false, nil
	}

	// Leader key pressed
	if s == h.LeaderKey {
		h.LeaderWaiting = true
		h.Buffer = []string{h.LeaderSeq}
		return true, nil
	}

	// In leader mode: append key and look up
	if h.LeaderWaiting {
		keyPart := keyToSeqPart(s)
		h.Buffer = append(h.Buffer, keyPart)
		seq := strings.Join(h.Buffer, " ")

		if c := h.Registry.Lookup(seq); c != nil {
			h.LeaderWaiting = false
			h.Buffer = nil
			return true, c
		}
		// No exact match; stay in leader mode if a longer binding exists
		if h.Registry.HasPrefix(seq) {
			return true, nil
		}
		h.LeaderWaiting = false
		h.Buffer = nil
		return true, nil
	}

	// Not in leader mode: check single-key bindings
	if c := h.Registry.Lookup(keyToSeqPart(s)); c != nil {
		return true, c
	}

	return false, nil
}

// keyToSeqPart converts a tea key string to our sequence part.
func keyToSeqPart(s string) string {
	if s == " " || s == "space" {
		return "SPC"
	}
	return s
}
