package ui

import (
	"fmt"
	"strings"

	"devdeploy/internal/agent"
	"devdeploy/internal/beads"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"
	"devdeploy/internal/session"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ralphPrompt is the canned prompt sent to an agent for automated work loops.
const ralphPrompt = "Run `bd ready` to see available work. Pick one issue, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the rules in .cursor/rules/."

// ralphTargetedPrompt returns a prompt that tells the agent to work on a specific bead.
func ralphTargetedPrompt(beadID string) string {
	return fmt.Sprintf("Run `bd show %s` to understand the issue. Claim it with `bd update %s --status in_progress`, implement it, then close it with `bd close %s`. Follow the rules in .cursor/rules/.", beadID, beadID, beadID)
}

// SelectProjectMsg is sent when user selects a project from the dashboard.
type SelectProjectMsg struct {
	Name string
}

// OpenShellMsg is sent when user opens a shell on the selected resource (SPC s s or Enter).
type OpenShellMsg struct{}

// LaunchAgentMsg is sent when user launches an agent on the selected resource (SPC s a).
type LaunchAgentMsg struct{}

// LaunchRalphMsg is sent when user launches a Ralph loop on the selected resource (SPC s r).
// Ralph is an automated agent that picks open work and implements it.
type LaunchRalphMsg struct{}

// HidePaneMsg hides the selected resource's most recent pane (break-pane to background window).
type HidePaneMsg struct{}

// ShowPaneMsg shows the selected resource's most recent pane (join-pane back into current window).
type ShowPaneMsg struct{}

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

// ShowRemoveResourceMsg triggers the remove-resource confirmation (project detail, 'd' key).
type ShowRemoveResourceMsg struct{}

// RemoveResourceMsg is sent when user confirms removal of a resource.
// Kills associated panes and removes the worktree.
type RemoveResourceMsg struct {
	ProjectName string
	Resource    project.Resource
}

// DismissModalMsg is sent when user cancels a modal (Esc).
type DismissModalMsg struct{}

// AppModel is the root model implementing Option E (Dashboard + Detail).
// It switches between Dashboard and ProjectDetail modes.
type AppModel struct {
	Mode            AppMode
	Dashboard       *DashboardView
	Detail          *ProjectDetailView
	KeyHandler      *KeyHandler
	ProjectManager  *project.Manager
	AgentRunner     agent.Runner
	Sessions        *session.Tracker // tracks panes across all resources; persists across project switches
	Overlays        OverlayStack
	Status          string // Error or success message; cleared on keypress
	StatusIsError   bool
	agentCancelFunc func() // cancels in-flight agent run; nil when none
	termWidth       int    // terminal width from last WindowSizeMsg
	termHeight      int    // terminal height from last WindowSizeMsg
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
	// Store terminal size so it can be passed to views created later.
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		a.termWidth = wsm.Width
		a.termHeight = wsm.Height
		if a.Detail != nil {
			a.Detail.SetSize(wsm.Width, wsm.Height)
		}
	}

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
			// Kill all panes for resources in this project before deleting.
			if a.Sessions != nil {
				resources := a.ProjectManager.ListProjectResources(msg.Name)
				for _, r := range resources {
					rk := resourceKeyFromResource(r)
					panes := a.Sessions.PanesForResource(rk)
					for _, p := range panes {
						_ = tmux.KillPane(p.PaneID)
					}
					a.Sessions.UnregisterAll(rk)
				}
			}
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
				a.Detail.Resources = a.ProjectManager.ListProjectResources(msg.ProjectName)
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
				a.Detail.Resources = a.ProjectManager.ListProjectResources(msg.ProjectName)
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
	case ShowRemoveResourceMsg:
		if a.Mode != ModeProjectDetail || a.Detail == nil {
			return a, nil
		}
		r := a.Detail.SelectedResource()
		if r == nil {
			a.Status = "No resource selected"
			a.StatusIsError = true
			return a, nil
		}
		modal := NewRemoveResourceConfirmModal(a.Detail.ProjectName, *r)
		a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
		return a, modal.Init()
	case RemoveResourceMsg:
		if a.ProjectManager == nil {
			return a, nil
		}
		// Kill associated tmux panes (best-effort; pane may already be dead).
		if a.Sessions != nil {
			rk := resourceKeyFromResource(msg.Resource)
			panes := a.Sessions.PanesForResource(rk)
			for _, p := range panes {
				_ = tmux.KillPane(p.PaneID) // ignore errors for dead panes
			}
			a.Sessions.UnregisterAll(rk)
		}
		// Remove worktree based on resource kind.
		var removeErr error
		switch msg.Resource.Kind {
		case project.ResourceRepo:
			removeErr = a.ProjectManager.RemoveRepo(msg.ProjectName, msg.Resource.RepoName)
		case project.ResourcePR:
			if msg.Resource.PR != nil {
				removeErr = a.ProjectManager.RemovePRWorktree(msg.ProjectName, msg.Resource.RepoName, msg.Resource.PR.Number)
			}
		}
		if removeErr != nil {
			a.Status = fmt.Sprintf("Remove resource: %v", removeErr)
			a.StatusIsError = true
		} else {
			label := msg.Resource.RepoName
			if msg.Resource.Kind == project.ResourcePR && msg.Resource.PR != nil {
				label = fmt.Sprintf("PR #%d (%s)", msg.Resource.PR.Number, msg.Resource.RepoName)
			}
			a.Status = fmt.Sprintf("Removed %s", label)
			a.StatusIsError = false
		}
		a.Overlays.Pop()
		// Refresh the resource list and pane info.
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
			a.Detail.Resources = a.ProjectManager.ListProjectResources(msg.ProjectName)
			// Clamp selection index.
			if a.Detail.Selected >= len(a.Detail.Resources) {
				a.Detail.Selected = len(a.Detail.Resources) - 1
			}
			if a.Detail.Selected < 0 {
				a.Detail.Selected = 0
			}
			a.refreshDetailPanes()
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
			}
		}
		a.Overlays.Pop()
		return a, nil
	case OpenShellMsg:
		if a.Mode != ModeProjectDetail || a.Detail == nil {
			return a, nil
		}
		r := a.Detail.SelectedResource()
		if r == nil {
			a.Status = "No resource selected"
			a.StatusIsError = true
			return a, nil
		}
		workDir, err := a.ensureResourceWorktree(r)
		if err != nil {
			a.Status = fmt.Sprintf("Open shell: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Open shell: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if a.Sessions != nil {
			rk := resourceKeyFromResource(*r)
			a.Sessions.Register(rk, paneID, session.PaneShell)
			a.refreshDetailPanes()
		}
		return a, nil
	case LaunchAgentMsg:
		if a.Mode != ModeProjectDetail || a.Detail == nil {
			return a, nil
		}
		r := a.Detail.SelectedResource()
		if r == nil {
			a.Status = "No resource selected"
			a.StatusIsError = true
			return a, nil
		}
		workDir, err := a.ensureResourceWorktree(r)
		if err != nil {
			a.Status = fmt.Sprintf("Launch agent: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Launch agent: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if err := tmux.SendKeys(paneID, "agent --model claude-4.5-opus-high-thinking\n"); err != nil {
			a.Status = fmt.Sprintf("Send agent command: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if a.Sessions != nil {
			rk := resourceKeyFromResource(*r)
			a.Sessions.Register(rk, paneID, session.PaneAgent)
			a.refreshDetailPanes()
		}
		return a, nil
	case LaunchRalphMsg:
		if a.Mode != ModeProjectDetail || a.Detail == nil {
			return a, nil
		}
		r := a.Detail.SelectedResource()
		if r == nil {
			a.Status = "No resource selected"
			a.StatusIsError = true
			return a, nil
		}
		if len(r.Beads) == 0 {
			a.Status = "No open beads for this resource"
			a.StatusIsError = true
			return a, nil
		}
		workDir, err := a.ensureResourceWorktree(r)
		if err != nil {
			a.Status = fmt.Sprintf("Ralph: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Ralph: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		// Use targeted prompt if cursor is on a specific bead, otherwise use generic prompt.
		prompt := ralphPrompt
		selectedBead := a.Detail.SelectedBead()
		if selectedBead != nil {
			prompt = ralphTargetedPrompt(selectedBead.ID)
		}
		// Pass the prompt as a single-quoted positional argument to agent.
		// Single quotes prevent the shell from interpreting backticks and $.
		escaped := strings.ReplaceAll(prompt, "'", `'\''`)
		cmd := fmt.Sprintf("agent --model composer-1 --force '%s'\n", escaped)
		if err := tmux.SendKeys(paneID, cmd); err != nil {
			a.Status = fmt.Sprintf("Ralph send agent: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if a.Sessions != nil {
			rk := resourceKeyFromResource(*r)
			a.Sessions.Register(rk, paneID, session.PaneAgent)
			a.refreshDetailPanes()
		}
		statusMsg := "Ralph loop launched"
		if selectedBead != nil {
			statusMsg = fmt.Sprintf("Ralph launched for %s", selectedBead.ID)
		}
		a.Status = statusMsg
		a.StatusIsError = false
		return a, nil
	case HidePaneMsg:
		paneID := a.selectedResourceLatestPaneID()
		if paneID == "" {
			a.Status = "No pane to hide"
			return a, nil
		}
		if err := tmux.BreakPane(paneID); err != nil {
			a.Status = fmt.Sprintf("Hide pane: %v", err)
			a.StatusIsError = true
		}
		return a, nil
	case ShowPaneMsg:
		paneID := a.selectedResourceLatestPaneID()
		if paneID == "" {
			a.Status = "No pane to show"
			return a, nil
		}
		if err := tmux.JoinPane(paneID); err != nil {
			a.Status = fmt.Sprintf("Show pane: %v", err)
			a.StatusIsError = true
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
		if a.Mode == ModeProjectDetail && msg.String() == "enter" {
			return a, func() tea.Msg { return OpenShellMsg{} }
		}
		if a.Mode == ModeProjectDetail && msg.String() == "d" {
			return a, func() tea.Msg { return ShowRemoveResourceMsg{} }
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

// newProjectDetailView creates a detail view with resources from disk/gh.
func (a *AppModel) newProjectDetailView(name string) *ProjectDetailView {
	v := NewProjectDetailView(name)
	if a.termWidth > 0 || a.termHeight > 0 {
		v.SetSize(a.termWidth, a.termHeight)
	}
	if a.ProjectManager != nil {
		v.Resources = a.ProjectManager.ListProjectResources(name)
	}
	// Populate beads from bd for each resource with a worktree.
	a.populateResourceBeads(v)
	// Prune dead panes then populate pane info from session tracker
	if a.Sessions != nil {
		a.Sessions.Prune()
		a.populateResourcePanes(v)
	}
	return v
}

// populateResourceBeads queries bd for beads associated with each resource
// and attaches them. Only resources with worktrees are queried (bd needs a
// working directory). Called once during view construction to avoid
// re-querying on every keypress.
func (a *AppModel) populateResourceBeads(v *ProjectDetailView) {
	for i := range v.Resources {
		r := &v.Resources[i]
		if r.WorktreePath == "" {
			continue
		}
		var bdBeads []beads.Bead
		switch r.Kind {
		case project.ResourceRepo:
			bdBeads = beads.ListForRepo(r.WorktreePath, v.ProjectName)
		case project.ResourcePR:
			if r.PR != nil {
				bdBeads = beads.ListForPR(r.WorktreePath, v.ProjectName, r.PR.Number)
			}
		}
		r.Beads = make([]project.BeadInfo, len(bdBeads))
		for j, b := range bdBeads {
			r.Beads[j] = project.BeadInfo{
				ID:        b.ID,
				Title:     b.Title,
				Status:    b.Status,
				IssueType: b.IssueType,
				IsChild:   b.ParentID != "",
			}
		}
	}
}

// populateResourcePanes attaches tracked pane info to each resource in the detail view.
func (a *AppModel) populateResourcePanes(v *ProjectDetailView) {
	if a.Sessions == nil {
		return
	}
	for i := range v.Resources {
		r := &v.Resources[i]
		rk := resourceKeyFromResource(*r)
		tracked := a.Sessions.PanesForResource(rk)
		r.Panes = nil
		for _, tp := range tracked {
			r.Panes = append(r.Panes, project.PaneInfo{
				ID:      tp.PaneID,
				IsAgent: tp.Type == session.PaneAgent,
			})
		}
	}
}

// refreshDetailPanes prunes dead panes then updates the current detail view's
// pane info from the session tracker.
func (a *AppModel) refreshDetailPanes() {
	if a.Detail != nil && a.Sessions != nil {
		a.Sessions.Prune()
		a.populateResourcePanes(a.Detail)
	}
}

// selectedResourceLatestPaneID returns the pane ID of the most recently registered
// pane for the currently selected resource, or "" if none.
func (a *AppModel) selectedResourceLatestPaneID() string {
	if a.Mode != ModeProjectDetail || a.Detail == nil || a.Sessions == nil {
		return ""
	}
	r := a.Detail.SelectedResource()
	if r == nil {
		return ""
	}
	rk := resourceKeyFromResource(*r)
	panes := a.Sessions.PanesForResource(rk)
	if len(panes) == 0 {
		return ""
	}
	return panes[len(panes)-1].PaneID
}

// ensureResourceWorktree returns the worktree path for a resource, creating
// a PR worktree if needed. For repo resources, it uses the existing WorktreePath.
// For PR resources with no worktree, it calls EnsurePRWorktree to create one
// and updates the resource's WorktreePath in the detail view.
// Rule injection is always attempted (idempotent) so worktrees created before
// the injection feature was added still get rules on first shell/agent open.
func (a *AppModel) ensureResourceWorktree(r *project.Resource) (string, error) {
	if r.WorktreePath != "" {
		_ = project.InjectWorktreeRules(r.WorktreePath)
		return r.WorktreePath, nil
	}
	if r.Kind != project.ResourcePR || r.PR == nil {
		return "", fmt.Errorf("no worktree for this resource")
	}
	if a.ProjectManager == nil || a.Detail == nil {
		return "", fmt.Errorf("no project manager available")
	}
	if r.PR.HeadRefName == "" {
		return "", fmt.Errorf("PR #%d has no branch name", r.PR.Number)
	}
	wtPath, err := a.ProjectManager.EnsurePRWorktree(
		a.Detail.ProjectName, r.RepoName, r.PR.Number, r.PR.HeadRefName,
	)
	if err != nil {
		return "", err
	}
	// Update the resource so subsequent actions reuse the worktree.
	r.WorktreePath = wtPath
	return wtPath, nil
}

// resourceKeyFromResource builds a session.ResourceKey from a project.Resource.
func resourceKeyFromResource(r project.Resource) string {
	if r.Kind == project.ResourcePR && r.PR != nil {
		return session.ResourceKey("pr", r.RepoName, r.PR.Number)
	}
	return session.ResourceKey("repo", r.RepoName, 0)
}

// loadProjectsCmd returns a command that loads projects from disk and sends ProjectsLoadedMsg.
// It uses LoadProjectSummary to fetch open PRs once per repo (instead of
// separate CountPRs + ListProjectResources calls which would invoke
// gh pr list up to 3N times per N repos).
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
			summary := m.LoadProjectSummary(info.Name)
			projects[i] = ProjectSummary{
				Name:      info.Name,
				RepoCount: info.RepoCount,
				PRCount:   summary.PRCount,
				BeadCount: countBeadsFromResources(summary.Resources, info.Name),
				Selected:  false, // Dashboard uses Selected index
			}
		}
		return ProjectsLoadedMsg{Projects: projects}
	}
}

// countBeadsFromResources counts open beads across the given resources.
// Used by loadProjectsCmd with resources from LoadProjectSummary to avoid
// a separate ListProjectResources call (which would redundantly fetch PRs).
func countBeadsFromResources(resources []project.Resource, projectName string) int {
	count := 0
	for _, r := range resources {
		if r.WorktreePath == "" {
			continue
		}
		switch r.Kind {
		case project.ResourceRepo:
			count += len(beads.ListForRepo(r.WorktreePath, projectName))
		case project.ResourcePR:
			if r.PR != nil {
				count += len(beads.ListForPR(r.WorktreePath, projectName, r.PR.Number))
			}
		}
	}
	return count
}

// NewAppModel creates the root application model.
func NewAppModel() *AppModel {
	projMgr := (*project.Manager)(nil)
	if base, err := project.ResolveProjectsBase(); err == nil {
		projMgr = project.NewManager(base, "")
	}
	reg := NewKeybindRegistry()
	reg.BindWithDesc("q", tea.Quit, "Quit")
	reg.BindWithDesc("ctrl+c", tea.Quit, "Quit")
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	reg.BindWithDescForMode("SPC s s", func() tea.Msg { return OpenShellMsg{} }, "Open shell", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC s a", func() tea.Msg { return LaunchAgentMsg{} }, "Launch agent", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC s r", func() tea.Msg { return LaunchRalphMsg{} }, "Ralph loop", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC s h", func() tea.Msg { return HidePaneMsg{} }, "Hide shell pane", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC s j", func() tea.Msg { return ShowPaneMsg{} }, "Show shell pane", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC p c", func() tea.Msg { return ShowCreateProjectMsg{} }, "Create project", []AppMode{ModeDashboard})
	reg.BindWithDescForMode("SPC p d", func() tea.Msg { return ShowDeleteProjectMsg{} }, "Delete project", []AppMode{ModeDashboard})
	reg.BindWithDescForMode("SPC p a", func() tea.Msg { return ShowAddRepoMsg{} }, "Add repo", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC p r", func() tea.Msg { return ShowRemoveRepoMsg{} }, "Remove repo", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC p x", func() tea.Msg { return ShowRemoveResourceMsg{} }, "Remove resource", []AppMode{ModeProjectDetail})
	return &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		Detail:         nil,
		KeyHandler:     NewKeyHandler(reg),
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
		Sessions:       session.New(tmux.ListPaneIDs),
	}
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
