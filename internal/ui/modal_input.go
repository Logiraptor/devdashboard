package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TextInputModal is a modal that prompts for a single text value.
// Enter submits; Esc cancels.
type TextInputModal struct {
	Title    string
	input    textinput.Model
	onSubmit func(value string) tea.Msg
	boxStyle lipgloss.Style
}

var _ View = (*TextInputModal)(nil)

// NewTextInputModal creates a text input modal with the given title and placeholder.
func NewTextInputModal(title, placeholder string, onSubmit func(string) tea.Msg) *TextInputModal {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 120
	ti.Width = 40

	return &TextInputModal{
		Title:    title,
		input:    ti,
		onSubmit: onSubmit,
		boxStyle: Styles.Box,
	}
}

func (m *TextInputModal) Init() tea.Cmd {
	return textinput.Blink
}

func (m *TextInputModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter":
			val := m.input.Value()
			if val == "" {
				return m, nil
			}
			if m.onSubmit != nil {
				return m, func() tea.Msg { return m.onSubmit(val) }
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *TextInputModal) View() string {
	content := Styles.Title.Render(m.Title) + "\n\n"
	content += m.input.View()
	content += "\n\n" + Styles.Hint.Render("Enter: confirm  Esc: cancel")
	return m.boxStyle.Render(content)
}
