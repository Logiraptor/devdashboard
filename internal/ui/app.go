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
	Home             *HomeView
	KeyHandler       *KeyHandler
	ProjectManager   *project.Manager
	AgentRunner      agent.Runner
	Sessions         *session.Tracker // tracks sessions across all resources
	DevdeploySession string           // tmux session name where devdeploy UI runs
	Overlays         OverlayStack
	Status           string // Error or success message; cleared on keypress
	StatusIsError    bool
	agentCancelFunc  func() // cancels in-flight agent run; nil when none
	termWidth        int    // terminal width from last WindowSizeMsg
	termHeight       int    // terminal height from last WindowSizeMsg
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
	case ShowAddWorktreeMsg:
		return a.handleShowAddWorktree()
	case AddWorktreeMsg:
		return a.handleAddWorktree(msg)
	case ShowRemoveResourceMsg:
		return a.handleShowRemoveResource()
	case RemoveResourceMsg:
		return a.handleRemoveResource(msg)
	case DismissModalMsg:
		return a.handleDismissModal()
	case RefreshMsg:
		return a.handleRefresh()
	case UpsertResourceSessionMsg:
		return a.handleUpsertResourceSession()
	case KillResourceSessionMsg:
		return a.handleKillResourceSession()
	case OpenCursorMsg:
		return a.handleOpenCursor()
	case tickMsg:
		return a.handleTick(msg)
	case homeTickRefreshedMsg:
		return a.handleHomeTickRefreshed(msg)
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
				return a, func() tea.Msg { return UpsertResourceSessionMsg{} }
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

// getGlobalSessionsForDisplay returns active sessions globally for display.
func (a *AppModel) getGlobalSessionsForDisplay() []project.SessionInfo {
	if a.Sessions == nil {
		return nil
	}
	tracked := a.getOrderedActiveSessions()
	out := make([]project.SessionInfo, len(tracked))
	for i, ts := range tracked {
		out[i] = project.SessionInfo{
			Name: ts.Name,
		}
	}
	return out
}

// populateHomeSessions attaches tracked session info to each resource in the home view.
func (a *AppModel) populateHomeSessions(v *HomeView) {
	if a.Sessions == nil {
		return
	}
	groups := v.RepoGroups()
	for gi := range groups {
		for ri := range groups[gi].Items {
			r := &groups[gi].Items[ri]
			rk := resourceKeyFromResource(*r)
			r.Session = nil
			if tracked, ok := a.Sessions.SessionForResource(rk); ok {
				r.Session = &project.SessionInfo{Name: tracked.Name}
			}
		}
	}
	v.SetRepoGroups(groups)
}

// refreshHomeSessions prunes dead sessions then updates the current home view.
func (a *AppModel) refreshHomeSessions() {
	if a.Home != nil && a.Sessions != nil {
		a.populateHomeSessions(a.Home)
	}
}

// selectedResourceSessionName returns tracked session name for selected resource.
func (a *AppModel) selectedResourceSessionName() string {
	if a.Home == nil || a.Sessions == nil {
		return ""
	}
	r := a.Home.SelectedResource()
	if r == nil {
		return ""
	}
	rk := resourceKeyFromResource(*r)
	if tracked, ok := a.Sessions.SessionForResource(rk); ok {
		return tracked.Name
	}
	return ""
}

// getOrderedActiveSessions returns active sessions ordered for display.
func (a *AppModel) getOrderedActiveSessions() []session.TrackedSession {
	if a.Sessions == nil {
		return nil
	}

	// Get all sessions globally across all resources.
	allSessions := a.Sessions.AllSessions()

	// Sort sessions: repos/worktrees first, then PRs, then by creation time.
	var repoSessions []session.TrackedSession
	var prSessions []session.TrackedSession

	for _, tracked := range allSessions {
		if tracked.ResourceKey.Kind() == "pr" {
			prSessions = append(prSessions, tracked)
		} else {
			repoSessions = append(repoSessions, tracked)
		}
	}

	sort.Slice(repoSessions, func(i, j int) bool {
		return repoSessions[i].CreatedAt.Before(repoSessions[j].CreatedAt)
	})
	sort.Slice(prSessions, func(i, j int) bool {
		return prSessions[i].CreatedAt.Before(prSessions[j].CreatedAt)
	})

	ordered := make([]session.TrackedSession, 0, len(repoSessions)+len(prSessions))
	ordered = append(ordered, repoSessions...)
	ordered = append(ordered, prSessions...)

	return ordered
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
	if r.Kind == project.ResourceWorktree && r.Worktree != nil {
		return session.NewWorktreeKey(r.RepoName, r.Worktree.Branch)
	}
	return session.NewRepoKey(r.RepoName)
}

// AppModelOption configures NewAppModel
type AppModelOption func(*AppModel)

// NewAppModel creates the root application model.
func NewAppModel(opts ...AppModelOption) *AppModel {
	projMgr := project.NewManager("", "")
	reg := NewKeybindRegistry()
	reg.BindWithDesc("q", tea.Quit, "Quit")
	reg.BindWithDesc("ctrl+c", tea.Quit, "Quit")
	reg.BindWithDesc("SPC q", tea.Quit, "Quit")
	reg.BindWithDesc("SPC s k", func() tea.Msg { return KillResourceSessionMsg{} }, "Kill resource session")
	reg.BindWithDesc("SPC s c", func() tea.Msg { return OpenCursorMsg{} }, "Open Cursor")
	reg.BindWithDesc("SPC p a", func() tea.Msg { return ShowAddWorktreeMsg{} }, "Add worktree")
	reg.BindWithDesc("SPC p x", func() tea.Msg { return ShowRemoveResourceMsg{} }, "Remove resource")
	reg.BindWithDesc("SPC r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads")
	reg.BindWithDesc("SPC b r", func() tea.Msg { return RefreshBeadsMsg{} }, "Refresh beads")
	reg.BindWithDesc("SPC b c", func() tea.Msg { return CloseBeadMsg{} }, "Close bead")
	devdeploySession := "devdeploy"
	if current, err := tmux.CurrentSessionName(); err == nil && current != "" {
		devdeploySession = current
	}
	model := &AppModel{
		KeyHandler:       NewKeyHandler(reg),
		ProjectManager:   projMgr,
		AgentRunner:      &agent.StubRunner{},
		Sessions:         session.New(tmux.ListSessionNames),
		DevdeploySession: devdeploySession,
	}

	// Apply options
	for _, opt := range opts {
		opt(model)
	}

	// Ensure home view has access to global session ordering from this model.
	if model.Home == nil {
		model.Home = NewHomeView()
	}
	model.Home.getGlobalSessions = model.getGlobalSessionsForDisplay

	return model
}

// AsTeaModel returns a tea.Model adapter for use with tea.NewProgram.
func (m *AppModel) AsTeaModel() tea.Model {
	return &appModelAdapter{AppModel: m}
}
