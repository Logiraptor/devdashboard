package ui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// RepoPickerModal is a modal for selecting a repo from a list.
type RepoPickerModal struct {
	list     list.Model
	items    []list.Item
	mode     repoPickerMode
	project  string
	onSelect func(project, repo string) tea.Msg
}

type repoPickerMode int

const (
	repoPickerAdd repoPickerMode = iota
	repoPickerRemove
)

type repoItem string

func (r repoItem) FilterValue() string { return string(r) }
func (r repoItem) Title() string       { return string(r) }
func (r repoItem) Description() string { return "" }

// Ensure RepoPickerModal implements View.
var _ View = (*RepoPickerModal)(nil)

// NewAddRepoModal creates a picker for adding a repo to a project.
func NewAddRepoModal(projectName string, repos []string) *RepoPickerModal {
	items := make([]list.Item, len(repos))
	for i, r := range repos {
		items[i] = repoItem(r)
	}
	return newRepoPickerModal(projectName, items, repoPickerAdd, func(p, r string) tea.Msg {
		return AddRepoMsg{ProjectName: p, RepoName: r}
	})
}

// NewRemoveRepoModal creates a picker for removing a repo from a project.
func NewRemoveRepoModal(projectName string, repos []string) *RepoPickerModal {
	items := make([]list.Item, len(repos))
	for i, r := range repos {
		items[i] = repoItem(r)
	}
	return newRepoPickerModal(projectName, items, repoPickerRemove, func(p, r string) tea.Msg {
		return RemoveRepoMsg{ProjectName: p, RepoName: r}
	})
}

func newRepoPickerModal(projectName string, items []list.Item, mode repoPickerMode, onSelect func(project, repo string) tea.Msg) *RepoPickerModal {
	title := "Add repo to " + projectName
	if mode == repoPickerRemove {
		title = "Remove repo from " + projectName
	}
	delegate := NewCompactListDelegate()
	l := list.New(items, delegate, 40, 12)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Styles.Title = Styles.Title
	return &RepoPickerModal{
		list:     l,
		items:    items,
		mode:     mode,
		project:  projectName,
		onSelect: onSelect,
	}
}

// Init implements View.
func (m *RepoPickerModal) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (m *RepoPickerModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter":
			if sel := m.list.SelectedItem(); sel != nil {
				repo := string(sel.(repoItem))
				return m, func() tea.Msg { return m.onSelect(m.project, repo) }
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements View.
func (m *RepoPickerModal) View() string {
	help := "Enter: select  Esc: cancel"
	return ModalStyles.BoxCompact.Render(m.list.View() + "\n" + ModalStyles.Help.Render(help))
}
