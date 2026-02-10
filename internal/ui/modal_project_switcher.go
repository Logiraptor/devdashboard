package ui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ProjectSwitcherModal is a modal for selecting a project from a list.
type ProjectSwitcherModal struct {
	list  list.Model
	items []list.Item
	names []string
}

type projectSwitcherItem string

func (p projectSwitcherItem) FilterValue() string { return string(p) }
func (p projectSwitcherItem) Title() string       { return string(p) }
func (p projectSwitcherItem) Description() string { return "" }

// Ensure ProjectSwitcherModal implements View.
var _ View = (*ProjectSwitcherModal)(nil)

// NewProjectSwitcherModal creates a picker for switching to a different project.
func NewProjectSwitcherModal(names []string) *ProjectSwitcherModal {
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = projectSwitcherItem(n)
	}
	delegate := NewCompactListDelegate()
	l := list.New(items, delegate, 40, 12)
	l.Title = "Switch project"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Styles.Title = Styles.Title
	return &ProjectSwitcherModal{
		list:  l,
		items: items,
		names: names,
	}
}

// Init implements View.
func (m *ProjectSwitcherModal) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (m *ProjectSwitcherModal) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return DismissModalMsg{} }
		case "enter":
			if sel := m.list.SelectedItem(); sel != nil {
				name := string(sel.(projectSwitcherItem))
				return m, func() tea.Msg { return SelectProjectMsg{Name: name} }
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements View.
func (m *ProjectSwitcherModal) View() string {
	help := "Enter: select  Esc: cancel"
	return ModalStyles.BoxCompact.Render(m.list.View() + "\n" + ModalStyles.Help.Render(help))
}
