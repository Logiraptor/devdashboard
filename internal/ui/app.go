package ui

import (
	"fmt"
	"sort"

	"devdeploy/internal/agent"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"
	"devdeploy/internal/session"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

// AppModel is the root model for the home-centric UI.
type AppModel struct {
	Home            *HomeView
	KeyHandler      *KeyHandler
	ProjectManager  *project.Manager
	AgentRunner     agent.Runner
	Sessions        *session.Tracker // tracks panes across all resources
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
		loadHomeRepoNamesCmd(a.ProjectManager),
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
		if a.Home != nil {
			a.Home.SetSize(wsm.Width, wsm.Height)
		}
	}

	switch msg := msg.(type) {
	case progress.Event:
		return a.handleProgressEvent(msg)
	case HomeRepoNamesLoadedMsg:
		return a.handleHomeRepoNamesLoaded(msg)
	case HomeRepoGroupsLoadedMsg:
		return a.handleHomeRepoGroupsLoaded(msg)
	case HomeBeadsLoadedMsg:
		return a.handleHomeBeadsLoaded(msg)
	case RefreshBeadsMsg:
		return a.handleRefreshBeads()
	case CloseBeadMsg:
		return a.handleCloseBead()
	case ShowRemoveResourceMsg:
		return a.handleShowRemoveResource()
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
		if msg.String() == "enter" {
			// Don't intercept enter if list is filtering - let it confirm the filter
			if a.Home != nil && a.Home.IsFiltering() {
				// Let it fall through to view update
			} else {
				return a, func() tea.Msg { return OpenShellMsg{} }
			}
		}
		if msg.String() == "d" {
			if a.Home != nil && a.Home.IsFiltering() {
				break
			}
			return a, func() tea.Msg { return ShowRemoveResourceMsg{} }
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
		mainView += "\n" + RenderKeybindHelp(a.KeyHandler)
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
	if a.Home != nil {
		return a.Home
	}
	return NewHomeView()
}

func (a *appModelAdapter) setCurrentView(v View) {
	if h, ok := v.(*HomeView); ok {
		a.Home = h
	}
}

// getGlobalPanesForDisplay returns all active panes globally as project.PaneInfo for display.
// Used by HomeView to show panes from all resources.
func (a *AppModel) getGlobalPanesForDisplay() []project.PaneInfo {
	if a.Sessions == nil {
		return nil
	}
	trackedPanes := a.getOrderedActivePanes()
	panes := make([]project.PaneInfo, len(trackedPanes))
	for i, tp := range trackedPanes {
		panes[i] = project.PaneInfo{
			ID:      tp.PaneID,
			IsAgent: tp.Type == session.PaneAgent,
		}
	}
	return panes
}

// populateHomePanes attaches tracked pane info to each resource in the home view.
func (a *AppModel) populateHomePanes(v *HomeView) {
	if a.Sessions == nil {
		return
	}
	groups := v.RepoGroups()
	for gi := range groups {
		for ri := range groups[gi].Items {
			r := &groups[gi].Items[ri]
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
	v.SetRepoGroups(groups)
}

// refreshHomePanes prunes dead panes then updates the current home view's
// pane info from the session tracker.
func (a *AppModel) refreshHomePanes() {
	if a.Home != nil && a.Sessions != nil {
		_, _ = a.Sessions.Prune() // ignore errors; cleanup is non-critical
		a.populateHomePanes(a.Home)
	}
}

// selectedResourceLatestPaneID returns the pane ID of the most recently registered
// pane for the currently selected resource, or "" if none.
func (a *AppModel) selectedResourceLatestPaneID() string {
	if a.Home == nil || a.Sessions == nil {
		return ""
	}
	r := a.Home.SelectedResource()
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
// Works globally from anywhere in devdeploy.
func (a *AppModel) getOrderedActivePanes() []session.TrackedPane {
	if a.Sessions == nil {
		return nil
	}

	// Prune dead panes first
	_, _ = a.Sessions.Prune() // ignore errors; cleanup is non-critical

	// Get all panes globally across all resources
	allPanes := a.Sessions.AllPanes()

	// Sort panes: repos first, then PRs, then by creation time within each group
	var repoPanes []session.TrackedPane
	var prPanes []session.TrackedPane

	for _, pane := range allPanes {
		if pane.ResourceKey.Kind() == "pr" {
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
func (a *AppModel) getPaneDisplayName(pane session.TrackedPane) string {
	var name string
	if pane.ResourceKey.Kind() == "pr" {
		// PR resource
		name = fmt.Sprintf("%s-pr-%d", pane.ResourceKey.RepoName(), pane.ResourceKey.PRNumber())
	} else {
		// Repo resource
		name = pane.ResourceKey.RepoName()
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
// and updates the resource's WorktreePath in the home view.
func (a *AppModel) ensureResourceWorktree(r *project.Resource) (string, error) {
	if r.WorktreePath != "" {
		return r.WorktreePath, nil
	}
	if r.Kind != project.ResourcePR || r.PR == nil {
		return "", fmt.Errorf("no worktree for this resource")
	}
	if a.ProjectManager == nil {
		return "", fmt.Errorf("no project manager available")
	}
	if r.PR.HeadRefName == "" {
		return "", fmt.Errorf("pr #%d has no branch name", r.PR.Number)
	}
	wtPath, err := a.ProjectManager.EnsurePRWorktree(
		"", r.RepoName, r.PR.Number, r.PR.HeadRefName,
	)
	if err != nil {
		return "", err
	}
	// Update the resource so subsequent actions reuse the worktree.
	r.WorktreePath = wtPath
	return wtPath, nil
}

// resourceKeyFromResource builds a session.ResourceKey from a project.Resource.
func resourceKeyFromResource(r project.Resource) session.ResourceKey {
	if r.Kind == project.ResourcePR && r.PR != nil {
		return session.NewPRKey(r.RepoName, r.PR.Number)
	}
	return session.NewRepoKey(r.RepoName)
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
	reg.BindWithDesc("SPC s s", func() tea.Msg { return OpenShellMsg{} }, "Open shell")
	reg.BindWithDesc("SPC s a", func() tea.Msg { return LaunchAgentMsg{} }, "Launch agent")
	reg.BindWithDesc("SPC s r", func() tea.Msg { return LaunchRalphMsg{} }, "Ralph loop")
	reg.BindWithDesc("SPC s h", func() tea.Msg { return HidePaneMsg{} }, "Hide shell pane")
	reg.BindWithDesc("SPC s j", func() tea.Msg { return ShowPaneMsg{} }, "Show shell pane")
	reg.BindWithDesc("SPC p x", func() tea.Msg { return ShowRemoveResourceMsg{} }, "Remove resource")
	reg.BindWithDesc("SPC r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads")
	reg.BindWithDesc("SPC b r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads")
	reg.BindWithDesc("SPC b c", func() tea.Msg { return CloseBeadMsg{} }, "Close bead")
	// SPC 1-9: focus pane by index
	for i := 1; i <= 9; i++ {
		num := i
		reg.BindWithDesc(
			fmt.Sprintf("SPC %d", i),
			func() tea.Msg { return FocusPaneMsg{Index: num} },
			fmt.Sprintf("Focus pane %d", i),
		)
	}
	model := &AppModel{
		KeyHandler:     NewKeyHandler(reg),
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
		Sessions:       session.New(tmux.ListPaneIDs),
	}

	// Apply options
	for _, opt := range opts {
		opt(model)
	}

	// Ensure home view has access to global pane ordering from this model.
	if model.Home == nil {
		model.Home = NewHomeView()
	}
	model.Home.getGlobalPanes = model.getGlobalPanesForDisplay

	return model
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
