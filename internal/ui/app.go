package ui

import tea "github.com/charmbracelet/bubbletea"

// SelectProjectMsg is sent when user selects a project from the dashboard.
type SelectProjectMsg struct {
	Name string
}

// AppModel is the root model implementing Option E (Dashboard + Detail).
// It switches between Dashboard and ProjectDetail modes.
type AppModel struct {
	Mode       AppMode
	Dashboard  *DashboardView
	Detail     *ProjectDetailView
	KeyHandler *KeyHandler
}

// Ensure AppModel can be used as tea.Model via adapter.
var _ tea.Model = (*appModelAdapter)(nil)

// appModelAdapter wraps AppModel to implement tea.Model.
type appModelAdapter struct {
	*AppModel
}

// Init implements tea.Model.
func (a *appModelAdapter) Init() tea.Cmd {
	return a.currentView().Init()
}

// Update implements tea.Model.
func (a *appModelAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SelectProjectMsg:
		a.Mode = ModeProjectDetail
		a.Detail = NewProjectDetailView(msg.Name)
		return a, a.Detail.Init()
	case tea.KeyMsg:
		// Keybind system (leader key, SPC-prefixed commands)
		if a.KeyHandler != nil {
			if consumed, keyCmd := a.KeyHandler.Handle(msg); consumed {
				return a, keyCmd
			}
		}
		// App-level navigation
		if a.Mode == ModeProjectDetail && msg.String() == "esc" {
			a.Mode = ModeDashboard
			a.Detail = nil
			return a, nil
		}
		if a.Mode == ModeDashboard && msg.String() == "enter" {
			d := a.Dashboard
			if d != nil && d.Selected < len(d.Projects) {
				return a, func() tea.Msg {
					return SelectProjectMsg{Name: d.Projects[d.Selected].Name}
				}
			}
		}
	}

	v, cmd := a.currentView().Update(msg)
	a.setCurrentView(v)
	return a, cmd
}

// View implements tea.Model.
func (a *appModelAdapter) View() string {
	return a.currentView().View()
}

func (a *appModelAdapter) currentView() View {
	switch a.Mode {
	case ModeDashboard:
		if a.Dashboard != nil {
			return a.Dashboard
		}
		return NewDashboardView()
	case ModeProjectDetail:
		if a.Detail != nil {
			return a.Detail
		}
	}
	return NewDashboardView()
}

func (a *appModelAdapter) setCurrentView(v View) {
	switch a.Mode {
	case ModeDashboard:
		if d, ok := v.(*DashboardView); ok {
			a.Dashboard = d
		}
	case ModeProjectDetail:
		if p, ok := v.(*ProjectDetailView); ok {
			a.Detail = p
		}
	}
}

// NewAppModel creates the root application model.
func NewAppModel() *AppModel {
	reg := NewKeybindRegistry()
	reg.Bind("q", tea.Quit)
	reg.Bind("ctrl+c", tea.Quit)
	reg.Bind("SPC q", tea.Quit) // spacemacs-style quit
	return &AppModel{
		Mode:       ModeDashboard,
		Dashboard:  NewDashboardView(),
		Detail:     nil,
		KeyHandler: NewKeyHandler(reg),
	}
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
