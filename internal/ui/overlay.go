package ui

import tea "github.com/charmbracelet/bubbletea"

// Overlay represents a modal or popup view with a dismiss key.
type Overlay struct {
	View   View
	Dismiss string // Key that dismisses (e.g. "esc")
}

// IsDismissKey returns true if the given key string should dismiss this overlay.
func (o *Overlay) IsDismissKey(key string) bool {
	return key == o.Dismiss
}

// OverlayStack manages a stack of overlays (topmost receives input first).
type OverlayStack struct {
	Stack []Overlay
}

// Push adds an overlay to the top of the stack.
func (s *OverlayStack) Push(o Overlay) {
	s.Stack = append(s.Stack, o)
}

// Pop removes and returns the top overlay.
func (s *OverlayStack) Pop() (Overlay, bool) {
	if len(s.Stack) == 0 {
		return Overlay{}, false
	}
	top := s.Stack[len(s.Stack)-1]
	s.Stack = s.Stack[:len(s.Stack)-1]
	return top, true
}

// Peek returns the top overlay without removing it.
func (s *OverlayStack) Peek() (Overlay, bool) {
	if len(s.Stack) == 0 {
		return Overlay{}, false
	}
	return s.Stack[len(s.Stack)-1], true
}

// Len returns the number of overlays in the stack.
func (s *OverlayStack) Len() int {
	return len(s.Stack)
}

// UpdateTop passes msg to the top overlay's Update and replaces its View with the result.
// Returns the cmd from the overlay's Update. Caller must run the cmd.
func (s *OverlayStack) UpdateTop(msg tea.Msg) (tea.Cmd, bool) {
	if len(s.Stack) == 0 {
		return nil, false
	}
	top := &s.Stack[len(s.Stack)-1]
	newView, cmd := top.View.Update(msg)
	top.View = newView
	return cmd, true
}
