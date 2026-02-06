package ui

import (
	"fmt"

	"devdeploy/internal/agent"
	"devdeploy/internal/artifact"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"
	"devdeploy/internal/pty"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectProjectMsg is sent when user selects a project from the dashboard.
type SelectProjectMsg struct {
	Name string
}

// RunAgentMsg is sent when user triggers agent run (SPC a a).
type RunAgentMsg struct{}

// ProjectsLoadedMsg is sent when projects are loaded from disk.
type ProjectsLoadedMsg struct {
	Projects []ProjectSummary
}

// CreateProjectMsg is sent when user creates a project (from modal).
type CreateProjectMsg struct {
	Name string
}

// DeleteProjectMsg is sent when user deletes the selected project.
type DeleteProjectMsg struct {
	Name string
}

// AddRepoMsg is sent when user adds a repo to a project (from picker).
type AddRepoMsg struct {
	ProjectName string
	RepoName    string
}

// RemoveRepoMsg is sent when user removes a repo from a project.
type RemoveRepoMsg struct {
	ProjectName string
	RepoName    string
}

// ShowCreateProjectMsg triggers the create-project modal.
type ShowCreateProjectMsg struct{}

// ShowDeleteProjectMsg triggers delete of the selected project (dashboard).
type ShowDeleteProjectMsg struct{}

// ShowAddRepoMsg triggers the add-repo picker (project detail).
type ShowAddRepoMsg struct{}

// ShowRemoveRepoMsg triggers the remove-repo picker (project detail).
type ShowRemoveRepoMsg struct{}

// DismissModalMsg is sent when user cancels a modal (Esc).
type DismissModalMsg struct{}

// AppModel is the root model implementing Option E (Dashboard + Detail).
// It switches between Dashboard and ProjectDetail modes.
type AppModel struct {
	Mode            AppMode
	Dashboard       *DashboardView
	Detail          *ProjectDetailView
	KeyHandler      *KeyHandler
	ArtifactStore   *artifact.Store
	ProjectManager  *project.Manager
	AgentRunner     agent.Runner
	PTYRunner       pty.Runner
	Overlays        OverlayStack
	Status          string // Error or success message; cleared on keypress
	StatusIsError   bool
	agentCancelFunc func() // cancels in-flight agent run; nil when none
}

// Ensure AppModel can be used as tea.Model via adapter.
var _ tea.Model = (*appModelAdapter)(nil)

// appModelAdapter wraps AppModel to implement tea.Model.
type appModelAdapter struct {
	*AppModel
}

// Init implements tea.Model.
func (a *appModelAdapter) Init() tea.Cmd {
	return tea.Batch(
		a.currentView().Init(),
		loadProjectsCmd(a.ProjectManager),
	)
}

// Update implements tea.Model.
func (a *appModelAdapter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case progress.Event:
		// Run finished (done or aborted); clear cancel so Esc just dismisses
		if msg.Status == progress.StatusDone || msg.Status == progress.StatusAborted {
			a.agentCancelFunc = nil
		}
		if a.Overlays.Len() > 0 {
			if top, hasOverlay := a.Overlays.Peek(); hasOverlay {
				if _, isProgress := top.View.(*ProgressWindow); isProgress {
					if cmd, updated := a.Overlays.UpdateTop(msg); updated {
						return a, cmd
					}
				}
			}
		}
		return a, nil
	case ProjectsLoadedMsg:
		if a.Dashboard != nil {
			a.Dashboard.Projects = msg.Projects
			a.Dashboard.Selected = 0
		}
		return a, nil
	case CreateProjectMsg:
		if a.ProjectManager != nil && msg.Name != "" {
			if err := a.ProjectManager.CreateProject(msg.Name); err != nil {
				a.Status = fmt.Sprintf("Create project: %v", err)
				a.StatusIsError = true
			} else {
				a.Status = "Project created"
				a.StatusIsError = false
			}
			a.Overlays.Pop()
			return a, loadProjectsCmd(a.ProjectManager)
		}
		return a, nil
	case DeleteProjectMsg:
		if a.ProjectManager != nil && msg.Name != "" {
			if err := a.ProjectManager.DeleteProject(msg.Name); err != nil {
				a.Status = fmt.Sprintf("Delete project: %v", err)
				a.StatusIsError = true
			} else {
				a.Status = "Project deleted"
				a.StatusIsError = false
			}
			a.Overlays.Pop()
			return a, loadProjectsCmd(a.ProjectManager)
		}
		return a, nil
	case AddRepoMsg:
		if a.ProjectManager != nil && msg.ProjectName != "" && msg.RepoName != "" {
			if err := a.ProjectManager.AddRepo(msg.ProjectName, msg.RepoName); err != nil {
				a.Status = fmt.Sprintf("Add repo: %v", err)
				a.StatusIsError = true
			} else {
				a.Status = fmt.Sprintf("Added %s to %s", msg.RepoName, msg.ProjectName)
				a.StatusIsError = false
			}
			if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
				repos, _ := a.ProjectManager.ListProjectRepos(msg.ProjectName)
				a.Detail.Repos = repos
			}
			a.Overlays.Pop()
			return a, nil
		}
		return a, nil
	case RemoveRepoMsg:
		if a.ProjectManager != nil && msg.ProjectName != "" && msg.RepoName != "" {
			if err := a.ProjectManager.RemoveRepo(msg.ProjectName, msg.RepoName); err != nil {
				a.Status = fmt.Sprintf("Remove repo: %v", err)
				a.StatusIsError = true
			} else {
				a.Status = fmt.Sprintf("Removed %s from %s", msg.RepoName, msg.ProjectName)
				a.StatusIsError = false
			}
			if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
				repos, _ := a.ProjectManager.ListProjectRepos(msg.ProjectName)
				a.Detail.Repos = repos
			}
			a.Overlays.Pop()
			return a, nil
		}
		return a, nil
	case ShowCreateProjectMsg:
		modal := NewCreateProjectModal()
		a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
		return a, modal.Init()
	case ShowDeleteProjectMsg:
		if a.Mode == ModeDashboard && a.Dashboard != nil && len(a.Dashboard.Projects) > 0 {
			idx := a.Dashboard.Selected
			if idx >= 0 && idx < len(a.Dashboard.Projects) {
				name := a.Dashboard.Projects[idx].Name
				modal := NewDeleteProjectConfirmModal(name)
				a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
				return a, modal.Init()
			}
		}
		return a, nil
	case ShowAddRepoMsg:
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.ProjectManager != nil {
			repos, err := a.ProjectManager.ListWorkspaceRepos()
			if err != nil {
				a.Status = fmt.Sprintf("List workspace repos: %v", err)
				a.StatusIsError = true
			} else if len(repos) == 0 {
				a.Status = "No repos found in ~/workspace (or DEVDEPLOY_WORKSPACE)"
				a.StatusIsError = true
			} else {
				a.Overlays.Push(Overlay{View: NewAddRepoModal(a.Detail.ProjectName, repos), Dismiss: "esc"})
			}
		}
		return a, nil
	case ShowRemoveRepoMsg:
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.ProjectManager != nil {
			repos, err := a.ProjectManager.ListProjectRepos(a.Detail.ProjectName)
			if err != nil {
				a.Status = fmt.Sprintf("List project repos: %v", err)
				a.StatusIsError = true
			} else if len(repos) == 0 {
				a.Status = "No repos in this project"
				a.StatusIsError = true
			} else {
				a.Overlays.Push(Overlay{View: NewRemoveRepoModal(a.Detail.ProjectName, repos), Dismiss: "esc"})
			}
		}
		return a, nil
	case DismissModalMsg:
		// If top overlay is ProgressWindow and we have an active agent run, cancel it
		// but keep the overlay visible so the user can see the "Aborted" state.
		// They press Esc again to dismiss after seeing it.
		if a.Overlays.Len() > 0 {
			if top, ok := a.Overlays.Peek(); ok {
				if _, isProgress := top.View.(*ProgressWindow); isProgress && a.agentCancelFunc != nil {
					a.agentCancelFunc()
					a.agentCancelFunc = nil
					return a, nil // Don't pop yet; user will see Aborted, then Esc again to dismiss
				}
				// ShellView: close PTY before popping
				if sv, isShell := top.View.(*ShellView); isShell {
					_ = sv.Close()
				}
			}
		}
		a.Overlays.Pop()
		return a, nil
	case RunAgentMsg:
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.ArtifactStore != nil {
			projectDir := a.ArtifactStore.ProjectDir(a.Detail.ProjectName)
			ptyRunner := a.PTYRunner
			if ptyRunner == nil {
				ptyRunner = &pty.CreackPTY{}
			}
			shellView := NewShellView(ptyRunner, projectDir)
			a.Overlays.Push(Overlay{View: shellView, Dismiss: "esc"})
			return a, shellView.Init()
		}
		return a, nil
	case SelectProjectMsg:
		a.Mode = ModeProjectDetail
		a.Detail = a.newProjectDetailView(msg.Name)
		return a, a.Detail.Init()
	case tea.KeyMsg:
		// When overlay is showing, it receives ALL keys first (no KeyHandler, no app nav).
		// This lets modals capture SPC, Esc, Enter, j/k etc. for text input and list navigation.
		if a.Overlays.Len() > 0 {
			if cmd, ok := a.Overlays.UpdateTop(msg); ok {
				return a, cmd
			}
			// Overlay consumed but returned nil cmd (e.g. typing in text input)
			return a, nil
		}
		// Clear status on any keypress (when no overlay)
		a.Status = ""
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

	// Non-KeyMsg with overlay (e.g. WindowSizeMsg) — pass to overlay
	if a.Overlays.Len() > 0 {
		if cmd, ok := a.Overlays.UpdateTop(msg); ok {
			return a, cmd
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
		base += "\n" + RenderKeybindHelp(a.KeyHandler, a.Mode)
	}
	if a.Overlays.Len() > 0 {
		if top, ok := a.Overlays.Peek(); ok {
			base += "\n" + top.View.View()
		}
	}
	if a.Status != "" {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
		if a.StatusIsError {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
		}
		base += "\n" + style.Render("▶ "+a.Status) + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(" (any key to dismiss)")
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

// newProjectDetailView creates a detail view with artifact content and repos from disk.
func (a *AppModel) newProjectDetailView(name string) *ProjectDetailView {
	v := NewProjectDetailView(name)
	if a.ArtifactStore != nil {
		art := a.ArtifactStore.Load(name)
		v.PlanContent = art.Plan
		v.DesignContent = art.Design
	}
	if a.ProjectManager != nil {
		repos, _ := a.ProjectManager.ListProjectRepos(name)
		v.Repos = repos
	}
	return v
}

// loadProjectsCmd returns a command that loads projects from disk and sends ProjectsLoadedMsg.
func loadProjectsCmd(m *project.Manager) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		infos, err := m.ListProjects()
		if err != nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		projects := make([]ProjectSummary, len(infos))
		for i, info := range infos {
			projects[i] = ProjectSummary{
				Name:      info.Name,
				RepoCount: info.RepoCount,
				PRCount:   m.CountPRs(info.Name),
				Artifacts: m.CountArtifacts(info.Name),
				Selected:  false, // Dashboard uses Selected index
			}
		}
		return ProjectsLoadedMsg{Projects: projects}
	}
}

// NewAppModel creates the root application model.
func NewAppModel() *AppModel {
	store, _ := artifact.NewStore() // ignore err; store nil = no artifacts
	projMgr := (*project.Manager)(nil)
	if store != nil {
		projMgr = project.NewManager(store.BaseDir(), "")
	}
	reg := NewKeybindRegistry()
	reg.BindWithDesc("q", tea.Quit, "Quit")
	reg.BindWithDesc("ctrl+c", tea.Quit, "Quit")
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	reg.BindWithDescForMode("SPC a a", func() tea.Msg { return RunAgentMsg{} }, "Agent run", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC p c", func() tea.Msg { return ShowCreateProjectMsg{} }, "Create project", []AppMode{ModeDashboard})
	reg.BindWithDescForMode("SPC p d", func() tea.Msg { return ShowDeleteProjectMsg{} }, "Delete project", []AppMode{ModeDashboard})
	reg.BindWithDescForMode("SPC p a", func() tea.Msg { return ShowAddRepoMsg{} }, "Add repo", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC p r", func() tea.Msg { return ShowRemoveRepoMsg{} }, "Remove repo", []AppMode{ModeProjectDetail})
	return &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		Detail:         nil,
		KeyHandler:     NewKeyHandler(reg),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
		PTYRunner:      &pty.CreackPTY{},
	}
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
