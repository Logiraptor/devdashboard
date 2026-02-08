package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// RemoveResourceConfirmModal asks the user to confirm removal of a resource.
// Enter or y confirms; Esc cancels.
type RemoveResourceConfirmModal struct {
	ProjectName string
	Resource    project.Resource
}

// Ensure RemoveResourceConfirmModal implements View.
var _ View = (*RemoveResourceConfirmModal)(nil)

// NewRemoveResourceConfirmModal creates a confirmation modal for removing a resource.
func NewRemoveResourceConfirmModal(projectName string, r project.Resource) *RemoveResourceConfirmModal {
	return &RemoveResourceConfirmModal{ProjectName: projectName, Resource: r}
}

// Init implements View.
func (m *RemoveResourceConfirmModal) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (m *RemoveResourceConfirmModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter", "y":
			return m, func() tea.Msg {
				return RemoveResourceMsg{
					ProjectName: m.ProjectName,
					Resource:    m.Resource,
				}
			}
		}
	}
	return m, nil
}

// View implements View.
func (m *RemoveResourceConfirmModal) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Margin(1)

	label := resourceLabel(m.Resource)
	var details string
	if m.Resource.WorktreePath != "" {
		details += "\nWorktree will be removed"
	}
	if len(m.Resource.Panes) > 0 {
		details += fmt.Sprintf("\n%d active pane(s) will be killed", len(m.Resource.Panes))
	}

	content := titleStyle.Render("Remove resource?") + "\n\n"
	content += lipgloss.NewStyle().Render(label)
	if details != "" {
		content += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(details)
	}
	content += "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("y/Enter: confirm  Esc: cancel")
	return boxStyle.Render(content)
}

// resourceLabel returns a human-readable label for a resource.
func resourceLabel(r project.Resource) string {
	switch r.Kind {
	case project.ResourcePR:
		if r.PR != nil {
			return fmt.Sprintf("PR #%d: %s (%s)", r.PR.Number, r.PR.Title, r.RepoName)
		}
		return fmt.Sprintf("PR (%s)", r.RepoName)
	default:
		return fmt.Sprintf("Repo: %s", r.RepoName)
	}
}
