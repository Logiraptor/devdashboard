package ui

import (
	"fmt"
	"sort"
	"strings"

	"devdeploy/internal/agent"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"
	"devdeploy/internal/session"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

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
	cmds := []tea.Cmd{
		a.currentView().Init(),
		loadProjectsCmd(a.ProjectManager),
		tickCmd(), // Start periodic refresh ticker
	}
	return tea.Batch(cmds...)
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
		return a.handleProgressEvent(msg)
	case ProjectsLoadedMsg:
		return a.handleProjectsLoaded(msg)
	case ProjectsEnrichedMsg:
		return a.handleProjectsEnriched(msg)
	case ProjectDetailResourcesLoadedMsg:
		return a.handleProjectDetailResourcesLoaded(msg)
	case ProjectPRsLoadedMsg:
		return a.handleProjectPRsLoaded(msg)
	case ProjectDetailPRsLoadedMsg:
		return a.handleProjectDetailPRsLoaded(msg)
	case ProjectDetailBeadsLoadedMsg:
		return a.handleProjectDetailBeadsLoaded(msg)
	case ResourceBeadsLoadedMsg:
		return a.handleResourceBeadsLoaded(msg)
	case RefreshBeadsMsg:
		return a.handleRefreshBeads()
	case CloseBeadMsg:
		return a.handleCloseBead()
	case CreateProjectMsg:
		return a.handleCreateProject(msg)
	case DeleteProjectMsg:
		return a.handleDeleteProject(msg)
	case AddRepoMsg:
		return a.handleAddRepo(msg)
	case RemoveRepoMsg:
		return a.handleRemoveRepo(msg)
	case ShowCreateProjectMsg:
		return a.handleShowCreateProject()
	case ShowDeleteProjectMsg:
		return a.handleShowDeleteProject()
	case ShowAddRepoMsg:
		return a.handleShowAddRepo()
	case ShowRemoveRepoMsg:
		return a.handleShowRemoveRepo()
	case ShowRemoveResourceMsg:
		return a.handleShowRemoveResource()
	case ShowProjectSwitcherMsg:
		return a.handleShowProjectSwitcher()
	case RemoveResourceMsg:
		return a.handleRemoveResource(msg)
	case DismissModalMsg:
		return a.handleDismissModal()
	case RefreshMsg:
		return a.handleRefresh()
	case OpenShellMsg:
		return a.handleOpenShell()
	case LaunchAgentMsg:
		return a.handleLaunchAgent()
	case LaunchRalphMsg:
		return a.handleLaunchRalph()
	case HidePaneMsg:
		return a.handleHidePane()
	case ShowPaneMsg:
		return a.handleShowPane()
	case FocusPaneMsg:
		return a.handleFocusPane(msg)
	case SelectProjectMsg:
		return a.handleSelectProject(msg)
	case tickMsg:
		return a.handleTick(msg)
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
			// Don't intercept esc if list is filtering - let it cancel the filter
			if a.Detail != nil && a.Detail.IsFiltering() {
				// Let it fall through to view update
			} else {
				a.Mode = ModeDashboard
				a.Detail = nil
				// Ensure Dashboard exists and initialize it to trigger re-render
				if a.Dashboard == nil {
					a.Dashboard = NewDashboardView()
				}
				return a, a.Dashboard.Init()
			}
		}
		if a.Mode == ModeProjectDetail && msg.String() == "enter" {
			// Don't intercept enter if list is filtering - let it confirm the filter
			if a.Detail != nil && a.Detail.IsFiltering() {
				// Let it fall through to view update
			} else {
				return a, func() tea.Msg { return OpenShellMsg{} }
			}
		}
		if a.Mode == ModeProjectDetail && msg.String() == "d" {
			return a, func() tea.Msg { return ShowRemoveResourceMsg{} }
		}
		if a.Mode == ModeDashboard && msg.String() == "enter" {
			d := a.Dashboard
			if d != nil {
				idx := d.Selected()
				if idx >= 0 && idx < len(d.Projects) {
					return a, func() tea.Msg {
						return SelectProjectMsg{Name: d.Projects[idx].Name}
					}
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
	mainView := a.currentView().View()

	// Add keybind help if leader waiting
	if a.KeyHandler != nil && a.KeyHandler.LeaderWaiting {
		mainView += "\n" + RenderKeybindHelp(a.KeyHandler, a.Mode)
	}

	// Add overlays
	if a.Overlays.Len() > 0 {
		if top, ok := a.Overlays.Peek(); ok {
			mainView += "\n" + top.View.View()
		}
	}

	// Add status
	if a.Status != "" {
		style := Styles.Status
		if a.StatusIsError {
			style = Styles.TitleWarning
		}
		mainView += "\n" + style.Render("▶ "+a.Status) + Styles.Muted.Render(" (any key to dismiss)")
	}

	return mainView
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

// getGlobalPanesForDisplay returns all active panes globally as project.PaneInfo for display.
// Used by ProjectDetailView to show panes from all projects.
func (a *AppModel) getGlobalPanesForDisplay() []project.PaneInfo {
	if a.Sessions == nil {
		return nil
	}
	// Get global panes using the same logic as getOrderedActivePanes
	trackedPanes := a.getOrderedActivePanes()
	// Convert to project.PaneInfo format
	panes := make([]project.PaneInfo, len(trackedPanes))
	for i, tp := range trackedPanes {
		panes[i] = project.PaneInfo{
			ID:      tp.PaneID,
			IsAgent: tp.Type == session.PaneAgent,
		}
	}
	return panes
}

// newProjectDetailView creates a detail view with resources from disk/gh.
// Returns the view and a command to start progressive loading.
func (a *AppModel) newProjectDetailView(name string) (*ProjectDetailView, tea.Cmd) {
	v := NewProjectDetailView(name)
	if a.termWidth > 0 || a.termHeight > 0 {
		v.SetSize(a.termWidth, a.termHeight)
	}
	// Set global panes getter so the view can show panes from all projects
	v.getGlobalPanes = a.getGlobalPanesForDisplay
	// Phase 1: Load repos instantly (filesystem-only, no GitHub API calls)
	var cmds []tea.Cmd
	if a.ProjectManager != nil {
		v.Resources = a.ProjectManager.ListProjectReposOnly(name)
		v.loadingPRs = true // PRs will be loaded asynchronously
		v.loadingBeads = false
		v.buildItems()                          // Build list items immediately so repos are visible
		cmds = append(cmds, v.spinnerTickCmd()) // Start spinner for PR loading
	}
	// Prune dead panes then populate pane info from session tracker
	if a.Sessions != nil {
		_, _ = a.Sessions.Prune() // ignore errors; cleanup is non-critical
		a.populateResourcePanes(v)
	}
	// Return command to trigger async enrichment (PRs, then beads)
	if a.ProjectManager != nil {
		cmds = append(cmds, loadProjectResourcesCmd(a.ProjectManager, name))
		return v, tea.Batch(cmds...)
	}
	if len(cmds) > 0 {
		return v, tea.Batch(cmds...)
	}
	return v, nil
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
		_, _ = a.Sessions.Prune() // ignore errors; cleanup is non-critical
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

// getOrderedActivePanes returns all active panes ordered for indexing (1-9).
// Panes are ordered by resource key (repos first, then PRs), then by creation time.
// Works globally from anywhere in devdeploy, not just from project detail view.
func (a *AppModel) getOrderedActivePanes() []session.TrackedPane {
	if a.Sessions == nil {
		return nil
	}

	// Prune dead panes first
	_, _ = a.Sessions.Prune() // ignore errors; cleanup is non-critical

	// Get all panes globally across all resources
	allPanes := a.Sessions.AllPanes()

	// Sort panes: repos first, then PRs, then by creation time within each group
	// Resource key format: "repo:name" or "pr:name:#number"
	var repoPanes []session.TrackedPane
	var prPanes []session.TrackedPane

	for _, pane := range allPanes {
		parts := strings.Split(pane.ResourceKey, ":")
		if len(parts) >= 2 && parts[0] == "pr" {
			prPanes = append(prPanes, pane)
		} else {
			repoPanes = append(repoPanes, pane)
		}
	}

	// Sort each group by creation time (oldest first for consistent ordering)
	sort.Slice(repoPanes, func(i, j int) bool {
		return repoPanes[i].CreatedAt.Before(repoPanes[j].CreatedAt)
	})
	sort.Slice(prPanes, func(i, j int) bool {
		return prPanes[i].CreatedAt.Before(prPanes[j].CreatedAt)
	})

	// Combine: repos first, then PRs
	ordered := make([]session.TrackedPane, 0, len(repoPanes)+len(prPanes))
	ordered = append(ordered, repoPanes...)
	ordered = append(ordered, prPanes...)

	// Limit to 9 panes for SPC 1-9
	if len(ordered) > 9 {
		ordered = ordered[:9]
	}

	return ordered
}

// getPaneDisplayName returns a human-readable name for a pane.
// Works globally without requiring Detail view.
func (a *AppModel) getPaneDisplayName(pane session.TrackedPane) string {
	// Parse resource key to get repo/PR info
	// Format: "repo:name" or "pr:name:#number"
	parts := strings.Split(pane.ResourceKey, ":")
	if len(parts) < 2 {
		return pane.PaneID
	}

	kind := parts[0]
	repoName := parts[1]

	var name string
	if kind == "pr" && len(parts) >= 3 {
		// PR resource: "pr:name:#number"
		prNum := strings.TrimPrefix(parts[2], "#")
		name = fmt.Sprintf("%s-pr-%s", repoName, prNum)
	} else {
		// Repo resource
		name = repoName
	}

	// Add pane type
	paneType := "shell"
	if pane.Type == session.PaneAgent {
		paneType = "agent"
	}

	return fmt.Sprintf("%s (%s)", name, paneType)
}

// ensureResourceWorktree returns the worktree path for a resource, creating
// a PR worktree if needed. For repo resources, it uses the existing WorktreePath.
// For PR resources with no worktree, it calls EnsurePRWorktree to create one
// and updates the resource's WorktreePath in the detail view.
// Rule injection is always attempted (idempotent) so worktrees created before
// the injection feature was added still get rules on first shell/agent open.
func (a *AppModel) ensureResourceWorktree(r *project.Resource) (string, error) {
	if r.WorktreePath != "" {
		// Ignore injection errors: rules are best-effort convenience for existing worktrees.
		// The worktree is usable even if rule injection fails.
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
		return "", fmt.Errorf("pr #%d has no branch name", r.PR.Number)
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
		return session.NewPRKey(r.RepoName, r.PR.Number).String()
	}
	return session.NewRepoKey(r.RepoName).String()
}

// AppModelOption configures NewAppModel
type AppModelOption func(*AppModel)

// NewAppModel creates the root application model.
func NewAppModel(opts ...AppModelOption) *AppModel {
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
	reg.BindWithDesc("SPC p l", func() tea.Msg { return ShowProjectSwitcherMsg{} }, "Switch project")
	// SPC r: refresh beads for all resources in project detail view
	reg.BindWithDescForMode("SPC r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads", []AppMode{ModeProjectDetail})
	// SPC b: bead operations
	reg.BindWithDescForMode("SPC b r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads", []AppMode{ModeProjectDetail})
	reg.BindWithDescForMode("SPC b c", func() tea.Msg { return CloseBeadMsg{} }, "Close bead", []AppMode{ModeProjectDetail})
	// SPC 1-9: focus pane by index
	for i := 1; i <= 9; i++ {
		num := i
		reg.BindWithDescForMode(
			fmt.Sprintf("SPC %d", i),
			func() tea.Msg { return FocusPaneMsg{Index: num} },
			fmt.Sprintf("Focus pane %d", i),
			[]AppMode{ModeProjectDetail},
		)
	}
	model := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		Detail:         nil,
		KeyHandler:     NewKeyHandler(reg),
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
		Sessions:       session.New(tmux.ListPaneIDs),
	}

	// Apply options
	for _, opt := range opts {
		opt(model)
	}

	return model
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
