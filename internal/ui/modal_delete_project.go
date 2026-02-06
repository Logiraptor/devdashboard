package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DeleteProjectConfirmModal asks the user to confirm deletion of a project.
// Enter or y confirms; Esc cancels.
type DeleteProjectConfirmModal struct {
	ProjectName string
}

// Ensure DeleteProjectConfirmModal implements View.
var _ View = (*DeleteProjectConfirmModal)(nil)

// NewDeleteProjectConfirmModal creates a confirmation modal for deleting a project.
func NewDeleteProjectConfirmModal(projectName string) *DeleteProjectConfirmModal {
	return &DeleteProjectConfirmModal{ProjectName: projectName}
}

// Init implements View.
func (m *DeleteProjectConfirmModal) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (m *DeleteProjectConfirmModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter", "y":
			return m, func() tea.Msg { return DeleteProjectMsg{Name: m.ProjectName} }
		}
	}
	return m, nil
}

// View implements View.
func (m *DeleteProjectConfirmModal) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Margin(1)
	content := titleStyle.Render("Delete project?") + "\n\n"
	content += lipgloss.NewStyle().Render("Project: "+m.ProjectName) + "\n\n"
	content += lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("y/Enter: confirm  Esc: cancel")
	return boxStyle.Render(content)
}
