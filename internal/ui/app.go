package ui

import (
	"context"
	"path/filepath"

	"devdeploy/internal/agent"
	"devdeploy/internal/artifact"
	"devdeploy/internal/progress"

	tea "github.com/charmbracelet/bubbletea"
)

// SelectProjectMsg is sent when user selects a project from the dashboard.
type SelectProjectMsg struct {
	Name string
}

// RunAgentMsg is sent when user triggers agent run (SPC a a).
type RunAgentMsg struct{}

// AppModel is the root model implementing Option E (Dashboard + Detail).
// It switches between Dashboard and ProjectDetail modes.
type AppModel struct {
	Mode          AppMode
	Dashboard     *DashboardView
	Detail        *ProjectDetailView
	KeyHandler    *KeyHandler
	ArtifactStore *artifact.Store
	AgentRunner   agent.Runner
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
	case progress.Event:
		// Phase 6 will display; for now we accept and discard
		return a, nil
	case RunAgentMsg:
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.AgentRunner != nil && a.ArtifactStore != nil {
			projectDir := a.ArtifactStore.ProjectDir(a.Detail.ProjectName)
			planPath := filepath.Join(projectDir, "plan.md")
			designPath := filepath.Join(projectDir, "design.md")
			return a, a.AgentRunner.Run(context.Background(), projectDir, planPath, designPath)
		}
		return a, nil
	case SelectProjectMsg:
		a.Mode = ModeProjectDetail
		a.Detail = a.newProjectDetailView(msg.Name)
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
	base := a.currentView().View()
	if a.KeyHandler != nil && a.KeyHandler.LeaderWaiting {
		base += "\n" + RenderKeybindHelp(a.KeyHandler.Registry)
	}
	return base
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

// newProjectDetailView creates a detail view with artifact content from the store.
func (a *AppModel) newProjectDetailView(name string) *ProjectDetailView {
	v := NewProjectDetailView(name)
	if a.ArtifactStore != nil {
		art := a.ArtifactStore.Load(name)
		v.PlanContent = art.Plan
		v.DesignContent = art.Design
	}
	return v
}

// NewAppModel creates the root application model.
func NewAppModel() *AppModel {
	store, _ := artifact.NewStore() // ignore err; store nil = no artifacts
	reg := NewKeybindRegistry()
	reg.BindWithDesc("q", tea.Quit, "Quit")
	reg.BindWithDesc("ctrl+c", tea.Quit, "Quit")
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	reg.BindWithDesc("SPC a a", func() tea.Msg { return RunAgentMsg{} }, "Agent run")
	return &AppModel{
		Mode:          ModeDashboard,
		Dashboard:     NewDashboardView(),
		Detail:        nil,
		KeyHandler:    NewKeyHandler(reg),
		ArtifactStore: store,
		AgentRunner:   &agent.StubRunner{},
	}
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
