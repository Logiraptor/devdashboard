package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// ConfirmModal is a generic confirmation modal that can be used for various actions.
// Enter or y confirms; Esc cancels.
type ConfirmModal struct {
	Title       string
	Label       string
	Details     string // Optional warning details (e.g., "Worktree will be removed")
	OnConfirm   func() tea.Msg
	boxStyle    lipgloss.Style
	titleStyle  lipgloss.Style
	detailStyle lipgloss.Style
	// For testing/debugging: store original data
	Resource project.Resource // Only set for RemoveResourceConfirmModal
}

// Ensure ConfirmModal implements View.
var _ View = (*ConfirmModal)(nil)

// NewConfirmModal creates a generic confirmation modal.
func NewConfirmModal(title, label string, onConfirm func() tea.Msg) *ConfirmModal {
	return &ConfirmModal{
		Title:       title,
		Label:       label,
		OnConfirm:   onConfirm,
		boxStyle:    ModalStyles.BoxWarning,
		titleStyle:  ModalStyles.TitleWarning,
		detailStyle: ModalStyles.Details,
	}
}

// WithDetails adds warning details to the modal.
func (m *ConfirmModal) WithDetails(details string) *ConfirmModal {
	m.Details = details
	return m
}

// NewDeleteProjectConfirmModal creates a confirmation modal for deleting a project.
func NewDeleteProjectConfirmModal(projectName string) *ConfirmModal {
	return NewConfirmModal(
		"Delete project?",
		fmt.Sprintf("Project: %s", projectName),
		func() tea.Msg { return DeleteProjectMsg{Name: projectName} },
	)
}

// NewRemoveResourceConfirmModal creates a confirmation modal for removing a resource.
func NewRemoveResourceConfirmModal(projectName string, r project.Resource) *ConfirmModal {
	label := resourceLabel(r)
	var details string
	if r.WorktreePath != "" {
		details += "\nWorktree will be removed"
	}
	if len(r.Panes) > 0 {
		details += fmt.Sprintf("\n%d active pane(s) will be killed", len(r.Panes))
	}
	
	modal := NewConfirmModal(
		"Remove resource?",
		label,
		func() tea.Msg {
			return RemoveResourceMsg{
				ProjectName: projectName,
				Resource:    r,
			}
		},
	)
	modal.Resource = r // Store for testing
	if details != "" {
		modal.WithDetails(details)
	}
	return modal
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

// Init implements View.
func (m *ConfirmModal) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (m *ConfirmModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter", "y":
			if m.OnConfirm != nil {
				return m, m.OnConfirm
			}
		}
	}
	return m, nil
}

// View implements View.
func (m *ConfirmModal) View() string {
	content := m.titleStyle.Render(m.Title) + "\n\n"
	content += ModalStyles.Label.Render(m.Label)
	if m.Details != "" {
		content += "\n" + m.detailStyle.Render(m.Details)
	}
	content += "\n\n" + ModalStyles.Help.Render("y/Enter: confirm  Esc: cancel")
	return m.boxStyle.Render(content)
}
