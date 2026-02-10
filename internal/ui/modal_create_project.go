package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// CreateProjectModal is a modal for entering a new project name.
type CreateProjectModal struct {
	input textinput.Model
}

// Ensure CreateProjectModal implements View.
var _ View = (*CreateProjectModal)(nil)

// NewCreateProjectModal creates a create-project modal.
func NewCreateProjectModal() *CreateProjectModal {
	ti := textinput.New()
	ti.Placeholder = "project-name"
	ti.Width = 40
	ti.Focus()
	return &CreateProjectModal{input: ti}
}

// Init implements View.
func (m *CreateProjectModal) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements View.
func (m *CreateProjectModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter":
			name := strings.TrimSpace(m.input.Value())
			if name != "" {
				return m, func() tea.Msg { return CreateProjectMsg{Name: name} }
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View implements View.
func (m *CreateProjectModal) View() string {
	content := ModalStyles.Title.Render("Create project") + "\n\n"
	content += m.input.View() + "\n\n"
	content += ModalStyles.Help.Render("Enter: create  Esc: cancel")
	return ModalStyles.BoxDefault.Render(content)
}
