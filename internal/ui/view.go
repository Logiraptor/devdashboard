package ui

import tea "github.com/charmbracelet/bubbletea"

// View is the unit of composition; implements Bubble Tea's Init/Update/View.
// Each View represents a screen or major UI region with its own model, update, and view.
type View interface {
	Init() tea.Cmd
	Update(tea.Msg) (View, tea.Cmd)
	View() string
}
