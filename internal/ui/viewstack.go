package ui

// ViewStack manages a stack of views for navigation (push/pop).
type ViewStack struct {
	Stack []View
}

// Push adds a view to the top of the stack.
func (s *ViewStack) Push(v View) {
	s.Stack = append(s.Stack, v)
}

// Pop removes and returns the top view.
// Returns nil if the stack is empty.
func (s *ViewStack) Pop() View {
	if len(s.Stack) == 0 {
		return nil
	}
	top := s.Stack[len(s.Stack)-1]
	s.Stack = s.Stack[:len(s.Stack)-1]
	return top
}

// Peek returns the top view without removing it.
func (s *ViewStack) Peek() View {
	if len(s.Stack) == 0 {
		return nil
	}
	return s.Stack[len(s.Stack)-1]
}

// Len returns the number of views in the stack.
func (s *ViewStack) Len() int {
	return len(s.Stack)
}
