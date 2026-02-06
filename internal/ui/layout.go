package ui

// Layout arranges panels and defines focus order.
type Layout interface {
	Panels() []Panel
	FocusOrder() []string // Tab order for focus
}
