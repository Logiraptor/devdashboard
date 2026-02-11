package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"devdeploy/internal/agent"
	"devdeploy/internal/beads"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"
	"devdeploy/internal/session"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

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

// FocusPaneMsg focuses a pane by index (1-9) from the list of active panes.
type FocusPaneMsg struct {
	Index int // 1-based index into active panes list
}

// ProjectsLoadedMsg is sent when projects are loaded from disk (phase 1: instant data).
// Contains project names and repo counts only (filesystem-only, <10ms).
type ProjectsLoadedMsg struct {
	Projects []ProjectSummary
}

// ProjectsEnrichedMsg is sent when PR and bead counts are loaded (phase 2: async data).
// Updates the projects with PR counts and bead counts that require network/subprocess calls.
type ProjectsEnrichedMsg struct {
	Projects []ProjectSummary
}

// ProjectDetailResourcesLoadedMsg is sent when project detail repos are loaded (phase 1: instant data).
// Contains repo resources only (filesystem-only, <10ms).
type ProjectDetailResourcesLoadedMsg struct {
	ProjectName string
	Resources   []project.Resource // repos only, no PRs or beads yet
}

// ProjectPRsLoadedMsg is sent when PRs are loaded for a project (phase 2: async data).
// Contains PRs grouped by repo, fetched in parallel.
type ProjectPRsLoadedMsg struct {
	ProjectName string
	PRsByRepo   []project.RepoPRs
}

// ProjectDetailPRsLoadedMsg is sent when PRs are loaded for a project detail view (phase 2: async data).
// Updates resources with PR information fetched in parallel across repos.
type ProjectDetailPRsLoadedMsg struct {
	ProjectName string
	Resources   []project.Resource // repos + PRs, no beads yet
}

// ProjectDetailBeadsLoadedMsg is sent when beads are loaded for a project detail view (phase 3: async data).
// Updates resources with bead information fetched in parallel across resources.
type ProjectDetailBeadsLoadedMsg struct {
	ProjectName string
	Resources   []project.Resource // repos + PRs + beads (complete)
}

// ResourceBeadsLoadedMsg is sent when beads are loaded for resources (phase 3: async data).
// Contains beads grouped by resource index for efficient attachment to existing resources.
type ResourceBeadsLoadedMsg struct {
	ProjectName     string
	BeadsByResource map[int][]project.BeadInfo // resource index -> beads
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

// ShowProjectSwitcherMsg triggers the project switcher modal.
type ShowProjectSwitcherMsg struct{}

// RefreshMsg triggers a manual refresh: clears PR cache and reloads current view.
type RefreshMsg struct{}

// RefreshBeadsMsg triggers a refresh of beads for all resources in the current project detail view.
type RefreshBeadsMsg struct{}

// RemoveResourceMsg is sent when user confirms removal of a resource.
// Kills associated panes and removes the worktree.
type RemoveResourceMsg struct {
	ProjectName string
	Resource    project.Resource
}

// DismissModalMsg is sent when user cancels a modal (Esc).
type DismissModalMsg struct{}

// tickMsg triggers periodic refresh of panes and beads.
type tickMsg time.Time

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

// handleProjectPRsLoaded handles ProjectPRsLoadedMsg by merging PRs into existing repo resources.
func (a *appModelAdapter) handleProjectPRsLoaded(msg ProjectPRsLoadedMsg) (tea.Model, tea.Cmd) {
	// Phase 2: PRs loaded, merge into existing repo resources
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		projDir := a.ProjectManager.ProjectDir(msg.ProjectName)

		// Build map of PRs by repo name for quick lookup
		repoPRsMap := make(map[string][]project.PRInfo)
		for _, repoPRs := range msg.PRsByRepo {
			repoPRsMap[repoPRs.Repo] = repoPRs.PRs
		}

		// Merge PR resources into existing repo resources
		resources := make([]project.Resource, 0, len(a.Detail.Resources))
		for _, repoRes := range a.Detail.Resources {
			// Add repo resource
			resources = append(resources, repoRes)

			// Add PR resources for this repo
			prs := repoPRsMap[repoRes.RepoName]
			for i := range prs {
				pr := &prs[i]
				prWT := filepath.Join(projDir, fmt.Sprintf("%s-pr-%d", repoRes.RepoName, pr.Number))
				var wtPath string
				if info, err := os.Stat(prWT); err == nil && info.IsDir() {
					wtPath = prWT
				}
				resources = append(resources, project.Resource{
					Kind:         project.ResourcePR,
					RepoName:     repoRes.RepoName,
					PR:           pr,
					WorktreePath: wtPath,
				})
			}
		}

		a.Detail.Resources = resources
		a.Detail.loadingPRs = false
		a.Detail.loadingBeads = true
		a.Detail.buildItems() // Rebuild list items to show PRs
		a.refreshDetailPanes()
		// Trigger Phase 3: Load beads asynchronously and start spinner
		return a, tea.Batch(
			a.Detail.spinnerTickCmd(),
			loadResourceBeadsCmd(msg.ProjectName, resources),
		)
	}
	return a, nil
}

// handleRemoveResource handles RemoveResourceMsg by killing panes and removing worktrees.
func (a *appModelAdapter) handleRemoveResource(msg RemoveResourceMsg) (tea.Model, tea.Cmd) {
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
	// Refresh the resource list and pane info via progressive reload.
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		// Trigger progressive reload
		return a, loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName)
	}
	return a, nil
}

// handleLaunchRalph handles LaunchRalphMsg by launching a Ralph loop or agent fallback.
func (a *appModelAdapter) handleLaunchRalph() (tea.Model, tea.Cmd) {
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
	// Check if ralph binary is available.
	ralphPath, err := exec.LookPath("ralph")
	if err != nil {
		// Fall back to agent-based approach if ralph not found.
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Ralph: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		// Use targeted prompt if cursor is on a specific bead, otherwise use generic prompt.
		prompt := "Run `bd ready` to see available work. Pick one issue, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the rules in .cursor/rules/."
		selectedBead := a.Detail.SelectedBead()
		if selectedBead != nil {
			// Branch to epic-aware flow if the selected bead is an epic
			if selectedBead.IssueType == "epic" {
				prompt = fmt.Sprintf("You are working on epic %s. Run `bd show %s` to understand the epic. Then use `bd ready --parent %s` to find its children. Process them sequentially: for each child, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the rules in .cursor/rules/ and AGENTS.md.", selectedBead.ID, selectedBead.ID, selectedBead.ID)
			} else {
				prompt = fmt.Sprintf("Run `bd show %s` to understand the issue. Claim it with `bd update %s --status in_progress`, implement it, then close it with `bd close %s`. Follow the rules in .cursor/rules/.", selectedBead.ID, selectedBead.ID, selectedBead.ID)
			}
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
		a.Status = "Ralph binary not found, using agent fallback"
		a.StatusIsError = false
		return a, nil
	}
	// Launch ralph binary with --workdir flag.
	// If a specific bead is selected, add --bead flag (or --epic if it's an epic).
	paneID, err := tmux.SplitPane(workDir)
	if err != nil {
		a.Status = fmt.Sprintf("Ralph: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	// Escape the workdir path for shell safety (handle spaces, special chars).
	escapedWorkdir := strings.ReplaceAll(workDir, "'", `'\''`)
	selectedBead := a.Detail.SelectedBead()
	cmd := fmt.Sprintf("%s --workdir '%s'", ralphPath, escapedWorkdir)
	
	// Determine if we're in epic mode (epics benefit from parallel processing)
	isEpicMode := selectedBead != nil && selectedBead.IssueType == "epic"
	
	// Add --max-parallel for concurrency on epics (default 3)
	// Single beads run sequentially (default maxParallel=1) to avoid conflicts
	if isEpicMode {
		cmd += " --max-parallel 3"
	}
	
	if selectedBead != nil {
		escapedBead := strings.ReplaceAll(selectedBead.ID, "'", `'\''`)
		// If selected bead is an epic, use --epic flag for parallel leaf processing
		if selectedBead.IssueType == "epic" {
			cmd += fmt.Sprintf(" --epic '%s'", escapedBead)
		} else {
			cmd += fmt.Sprintf(" --bead '%s'", escapedBead)
		}
	}
	cmd += "\n"
	if err := tmux.SendKeys(paneID, cmd); err != nil {
		a.Status = fmt.Sprintf("Ralph launch: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		rk := resourceKeyFromResource(*r)
		a.Sessions.Register(rk, paneID, session.PaneAgent)
		a.refreshDetailPanes()
	}
	// User can see ralph output directly in tmux pane
	a.Status = "Ralph loop launched"
	a.StatusIsError = false
	return a, nil
}

// handleSelectProject handles SelectProjectMsg by switching to project detail view.
func (a *appModelAdapter) handleSelectProject(msg SelectProjectMsg) (tea.Model, tea.Cmd) {
	// Pop overlay if present (e.g., from project switcher modal)
	if a.Overlays.Len() > 0 {
		a.Overlays.Pop()
	}
	a.Mode = ModeProjectDetail
	detail, cmd := a.newProjectDetailView(msg.Name)
	a.Detail = detail
	return a, tea.Batch(a.Detail.Init(), cmd, tickCmd()) // Start ticker when entering detail mode
}

// handleShowCreateProject handles ShowCreateProjectMsg by showing the create project modal.
func (a *appModelAdapter) handleShowCreateProject() (tea.Model, tea.Cmd) {
	modal := NewCreateProjectModal()
	a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
	return a, modal.Init()
}

// handleShowDeleteProject handles ShowDeleteProjectMsg by showing the delete confirmation modal.
func (a *appModelAdapter) handleShowDeleteProject() (tea.Model, tea.Cmd) {
	if a.Mode == ModeDashboard && a.Dashboard != nil && len(a.Dashboard.Projects) > 0 {
		idx := a.Dashboard.Selected()
		if idx >= 0 && idx < len(a.Dashboard.Projects) {
			name := a.Dashboard.Projects[idx].Name
			modal := NewDeleteProjectConfirmModal(name)
			a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
			return a, modal.Init()
		}
	}
	return a, nil
}

// handleShowAddRepo handles ShowAddRepoMsg by showing the add repo picker modal.
func (a *appModelAdapter) handleShowAddRepo() (tea.Model, tea.Cmd) {
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
}

// handleShowRemoveRepo handles ShowRemoveRepoMsg by showing the remove repo picker modal.
func (a *appModelAdapter) handleShowRemoveRepo() (tea.Model, tea.Cmd) {
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
}

// handleShowRemoveResource handles ShowRemoveResourceMsg by showing the remove resource confirmation modal.
func (a *appModelAdapter) handleShowRemoveResource() (tea.Model, tea.Cmd) {
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
}

// handleShowProjectSwitcher handles ShowProjectSwitcherMsg by showing the project switcher modal.
func (a *appModelAdapter) handleShowProjectSwitcher() (tea.Model, tea.Cmd) {
	if a.ProjectManager != nil {
		infos, err := a.ProjectManager.ListProjects()
		if err != nil {
			a.Status = fmt.Sprintf("List projects: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if len(infos) == 0 {
			a.Status = "No projects found"
			a.StatusIsError = true
			return a, nil
		}
		names := make([]string, len(infos))
		for i, info := range infos {
			names[i] = info.Name
		}
		modal := NewProjectSwitcherModal(names)
		a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
		return a, modal.Init()
	}
	return a, nil
}

// handleProjectsLoaded handles ProjectsLoadedMsg by updating the dashboard and triggering enrichment.
func (a *appModelAdapter) handleProjectsLoaded(msg ProjectsLoadedMsg) (tea.Model, tea.Cmd) {
	if a.Dashboard != nil {
		a.Dashboard.Projects = msg.Projects
		a.Dashboard.updateProjects()
		a.Dashboard.list.Select(0)
	}
	// Trigger async enrichment for PR and bead counts.
	if a.ProjectManager != nil && len(msg.Projects) > 0 {
		infos, _ := a.ProjectManager.ListProjects()
		// Match project infos to loaded projects to preserve repo counts.
		projectInfos := make([]project.ProjectInfo, 0, len(msg.Projects))
		for _, p := range msg.Projects {
			for _, info := range infos {
				if info.Name == p.Name {
					projectInfos = append(projectInfos, info)
					break
				}
			}
		}
		// Start spinner for async loading
		if a.Dashboard != nil {
			return a, tea.Batch(
				a.Dashboard.SetLoading(true),
				enrichesProjectsCmd(a.ProjectManager, projectInfos),
			)
		}
		return a, enrichesProjectsCmd(a.ProjectManager, projectInfos)
	}
	return a, nil
}

// handleProjectsEnriched handles ProjectsEnrichedMsg by updating the dashboard with enriched data.
func (a *appModelAdapter) handleProjectsEnriched(msg ProjectsEnrichedMsg) (tea.Model, tea.Cmd) {
	if a.Dashboard != nil {
		// Update projects with enriched data (PR and bead counts).
		// Preserve selection state.
		selectedIdx := a.Dashboard.Selected()
		a.Dashboard.Projects = msg.Projects
		a.Dashboard.updateProjects()
		if selectedIdx < len(msg.Projects) {
			a.Dashboard.list.Select(selectedIdx)
		}
		// Stop spinner
		return a, a.Dashboard.SetLoading(false)
	}
	return a, nil
}

// handleProjectDetailResourcesLoaded handles ProjectDetailResourcesLoadedMsg by updating resources and triggering PR loading.
func (a *appModelAdapter) handleProjectDetailResourcesLoaded(msg ProjectDetailResourcesLoadedMsg) (tea.Model, tea.Cmd) {
	// Phase 1: Repos loaded (for reload scenarios), update view and trigger PR loading
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		a.Detail.Resources = msg.Resources
		a.Detail.loadingPRs = true
		a.Detail.loadingBeads = false
		a.Detail.buildItems() // Rebuild list items
		a.refreshDetailPanes()
		// Trigger Phase 2: Load PRs asynchronously and start spinner
		if a.ProjectManager != nil {
			return a, tea.Batch(
				a.Detail.spinnerTickCmd(),
				loadProjectPRsCmd(a.ProjectManager, msg.ProjectName),
			)
		}
		return a, a.Detail.spinnerTickCmd()
	}
	return a, nil
}

// handleProjectDetailPRsLoaded handles ProjectDetailPRsLoadedMsg by updating resources and triggering bead loading.
func (a *appModelAdapter) handleProjectDetailPRsLoaded(msg ProjectDetailPRsLoadedMsg) (tea.Model, tea.Cmd) {
	// Phase 2: PRs loaded, update view and trigger bead loading
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		a.Detail.Resources = msg.Resources
		a.Detail.loadingPRs = false
		a.Detail.loadingBeads = true
		a.Detail.buildItems() // Rebuild list items to show PRs
		a.refreshDetailPanes()
		// Trigger Phase 3: Load beads asynchronously and start spinner
		return a, tea.Batch(
			a.Detail.spinnerTickCmd(),
			loadResourceBeadsCmd(msg.ProjectName, msg.Resources),
		)
	}
	return a, nil
}

// handleProjectDetailBeadsLoaded handles ProjectDetailBeadsLoadedMsg by updating resources with complete data.
func (a *appModelAdapter) handleProjectDetailBeadsLoaded(msg ProjectDetailBeadsLoadedMsg) (tea.Model, tea.Cmd) {
	// Phase 3: Beads loaded, update view (complete)
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		a.Detail.Resources = msg.Resources
		a.Detail.loadingPRs = false
		a.Detail.loadingBeads = false
		a.Detail.buildItems() // Rebuild list items to show beads
		a.refreshDetailPanes()
	}
	return a, nil
}

// handleResourceBeadsLoaded handles ResourceBeadsLoadedMsg by attaching beads to matching resources.
func (a *appModelAdapter) handleResourceBeadsLoaded(msg ResourceBeadsLoadedMsg) (tea.Model, tea.Cmd) {
	// Phase 3: Beads loaded, attach to matching resources in Detail view
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
		// Attach beads to matching resources by index
		for idx, beads := range msg.BeadsByResource {
			if idx >= 0 && idx < len(a.Detail.Resources) {
				a.Detail.Resources[idx].Beads = beads
			}
		}
		a.Detail.loadingBeads = false
		a.Detail.buildItems() // Rebuild list items to show beads
		a.refreshDetailPanes()
	}
	return a, nil
}

// handleRefreshBeads handles RefreshBeadsMsg by refreshing beads for all resources.
func (a *appModelAdapter) handleRefreshBeads() (tea.Model, tea.Cmd) {
	// Refresh beads for all resources in project detail view
	if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName != "" {
		a.Detail.loadingBeads = true
		a.Detail.buildItems() // Rebuild list items to show loading state
		a.refreshDetailPanes()
		// Trigger async bead loading
		return a, tea.Batch(
			a.Detail.spinnerTickCmd(),
			loadResourceBeadsCmd(a.Detail.ProjectName, a.Detail.Resources),
		)
	}
	return a, nil
}

// handleCreateProject handles CreateProjectMsg by creating a project and reloading the list.
func (a *appModelAdapter) handleCreateProject(msg CreateProjectMsg) (tea.Model, tea.Cmd) {
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
}

// handleDeleteProject handles DeleteProjectMsg by deleting a project and cleaning up panes.
func (a *appModelAdapter) handleDeleteProject(msg DeleteProjectMsg) (tea.Model, tea.Cmd) {
	if a.ProjectManager != nil && msg.Name != "" {
		// Kill all panes for resources in this project before deleting.
		if a.Sessions != nil {
			resources := a.ProjectManager.ListProjectResources(msg.Name)
			for _, r := range resources {
				rk := resourceKeyFromResource(r)
				panes := a.Sessions.PanesForResource(rk)
				for _, p := range panes {
					_ = tmux.KillPane(p.PaneID) // ignore errors for dead panes
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
}

// handleAddRepo handles AddRepoMsg by adding a repo to a project.
func (a *appModelAdapter) handleAddRepo(msg AddRepoMsg) (tea.Model, tea.Cmd) {
	if a.ProjectManager != nil && msg.ProjectName != "" && msg.RepoName != "" {
		if err := a.ProjectManager.AddRepo(msg.ProjectName, msg.RepoName); err != nil {
			a.Status = fmt.Sprintf("Add repo: %v", err)
			a.StatusIsError = true
		} else {
			a.Status = fmt.Sprintf("Added %s to %s", msg.RepoName, msg.ProjectName)
			a.StatusIsError = false
		}
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
			// Trigger progressive reload
			return a, tea.Batch(
				loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName),
			)
		}
		a.Overlays.Pop()
		return a, nil
	}
	return a, nil
}

// handleRemoveRepo handles RemoveRepoMsg by removing a repo from a project.
func (a *appModelAdapter) handleRemoveRepo(msg RemoveRepoMsg) (tea.Model, tea.Cmd) {
	if a.ProjectManager != nil && msg.ProjectName != "" && msg.RepoName != "" {
		if err := a.ProjectManager.RemoveRepo(msg.ProjectName, msg.RepoName); err != nil {
			a.Status = fmt.Sprintf("Remove repo: %v", err)
			a.StatusIsError = true
		} else {
			a.Status = fmt.Sprintf("Removed %s from %s", msg.RepoName, msg.ProjectName)
			a.StatusIsError = false
		}
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
			// Trigger progressive reload
			return a, tea.Batch(
				loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName),
			)
		}
		a.Overlays.Pop()
		return a, nil
	}
	return a, nil
}

// handleDismissModal handles DismissModalMsg by dismissing modals, with special handling for progress windows.
func (a *appModelAdapter) handleDismissModal() (tea.Model, tea.Cmd) {
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
}

// handleRefresh handles RefreshMsg by clearing PR cache and reloading the current view.
func (a *appModelAdapter) handleRefresh() (tea.Model, tea.Cmd) {
	// Clear PR cache
	if a.ProjectManager != nil {
		a.ProjectManager.ClearPRCache()
	}
	// Reload current view
	if a.Mode == ModeDashboard {
		// Reload dashboard
		return a, loadProjectsCmd(a.ProjectManager)
	} else if a.Mode == ModeProjectDetail && a.Detail != nil && a.ProjectManager != nil {
		// Reload project detail
		return a, loadProjectDetailResourcesCmd(a.ProjectManager, a.Detail.ProjectName)
	}
	return a, nil
}

// handleOpenShell handles OpenShellMsg by opening a shell pane for the selected resource.
func (a *appModelAdapter) handleOpenShell() (tea.Model, tea.Cmd) {
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
}

// handleLaunchAgent handles LaunchAgentMsg by launching an agent pane for the selected resource.
func (a *appModelAdapter) handleLaunchAgent() (tea.Model, tea.Cmd) {
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
	if err := tmux.SendKeys(paneID, "agent --model claude-4.5-opus-high-thinking --force\n"); err != nil {
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
}

// handleHidePane handles HidePaneMsg by hiding the selected resource's latest pane.
func (a *appModelAdapter) handleHidePane() (tea.Model, tea.Cmd) {
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
}

// handleShowPane handles ShowPaneMsg by showing the selected resource's latest pane.
func (a *appModelAdapter) handleShowPane() (tea.Model, tea.Cmd) {
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
}

// handleFocusPane handles FocusPaneMsg by focusing a pane by index.
func (a *appModelAdapter) handleFocusPane(msg FocusPaneMsg) (tea.Model, tea.Cmd) {
	if a.Sessions == nil {
		a.Status = "No session tracker"
		a.StatusIsError = true
		return a, nil
	}
	// Get ordered list of active panes
	panes := a.getOrderedActivePanes()
	if msg.Index < 1 || msg.Index > len(panes) {
		a.Status = fmt.Sprintf("Pane %d not available (1-%d)", msg.Index, len(panes))
		a.StatusIsError = true
		return a, nil
	}
	// Index is 1-based, convert to 0-based
	pane := panes[msg.Index-1]
	if err := tmux.FocusPaneAsSidebar(pane.PaneID); err != nil {
		a.Status = fmt.Sprintf("Focus pane: %v", err)
		a.StatusIsError = true
	} else {
		// Generate pane name for status
		paneName := a.getPaneDisplayName(pane)
		a.Status = fmt.Sprintf("Focused pane %d: %s", msg.Index, paneName)
		a.StatusIsError = false
	}
	return a, nil
}

// handleTick handles tickMsg by refreshing panes and beads periodically.
func (a *appModelAdapter) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	// Periodic refresh: update panes and beads when in project detail mode
	if a.Mode == ModeProjectDetail && a.Detail != nil {
		// Refresh panes (fast, local operation)
		a.refreshDetailPanes()

		// Refresh beads (slower, runs bd command)
		// Only refresh if we have resources with worktrees
		if a.ProjectManager != nil && len(a.Detail.Resources) > 0 {
			hasWorktrees := false
			for _, r := range a.Detail.Resources {
				if r.WorktreePath != "" {
					hasWorktrees = true
					break
				}
			}
			if hasWorktrees {
				return a, tea.Batch(
					loadResourceBeadsCmd(a.Detail.ProjectName, a.Detail.Resources),
					tickCmd(), // Schedule next tick
				)
			}
		}
		return a, tickCmd()
	}
	return a, tickCmd()
}

// handleProgressEvent handles progress.Event by updating progress windows and clearing cancel func.
func (a *appModelAdapter) handleProgressEvent(msg progress.Event) (tea.Model, tea.Cmd) {
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
		cmds = append(cmds, loadProjectPRsCmd(a.ProjectManager, name))
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
	var ordered []session.TrackedPane
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

// loadProjectsCmd returns a command that loads projects from disk (phase 1: instant data).
// It loads project names and repo counts from filesystem only (<10ms), then triggers
// async enrichment for PR and bead counts. Returns both ProjectsLoadedMsg (instant)
// and enrichesProjectsCmd (async) for progressive loading.
func loadProjectsCmd(m *project.Manager) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		infos, err := m.ListProjects()
		if err != nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		// Phase 1: Instant data (filesystem-only)
		projects := make([]ProjectSummary, len(infos))
		for i, info := range infos {
			projects[i] = ProjectSummary{
				Name:      info.Name,
				RepoCount: info.RepoCount,
				PRCount:   -1, // -1 indicates loading/unknown
				BeadCount: -1, // -1 indicates loading/unknown
				Selected:  false,
			}
		}
		return ProjectsLoadedMsg{Projects: projects}
	}
}

// enrichesProjectsCmd returns a command that enriches projects with PR and bead counts (phase 2: async data).
// This runs after the dashboard has rendered with instant data, fetching PRs and beads
// in parallel across repos and resources for optimal performance.
func enrichesProjectsCmd(m *project.Manager, projectInfos []project.ProjectInfo) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectsEnrichedMsg{Projects: nil}
		}
		projects := make([]ProjectSummary, len(projectInfos))

		// Parallelize across projects (each project's data is independent).
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i, info := range projectInfos {
			wg.Add(1)
			go func(idx int, projectName string, repoCount int) {
				defer wg.Done()
				summary := m.LoadProjectSummary(projectName)
				beadCount := countBeadsFromResources(summary.Resources, projectName)

				mu.Lock()
				projects[idx] = ProjectSummary{
					Name:      projectName,
					RepoCount: repoCount,
					PRCount:   summary.PRCount,
					BeadCount: beadCount,
					Selected:  false,
				}
				mu.Unlock()
			}(i, info.Name, info.RepoCount)
		}

		wg.Wait()

		return ProjectsEnrichedMsg{Projects: projects}
	}
}

// loadProjectDetailResourcesCmd returns a command that loads repos instantly (phase 1: instant data).
// This is filesystem-only and returns immediately with repo resources only.
func loadProjectDetailResourcesCmd(m *project.Manager, projectName string) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectDetailResourcesLoadedMsg{ProjectName: projectName, Resources: nil}
		}
		repos, _ := m.ListProjectRepos(projectName)
		projDir := m.ProjectDir(projectName)

		// Phase 1: Instant data (filesystem-only, no network calls)
		resources := make([]project.Resource, 0, len(repos))
		for _, repoName := range repos {
			worktreePath := filepath.Join(projDir, repoName)
			resources = append(resources, project.Resource{
				Kind:         project.ResourceRepo,
				RepoName:     repoName,
				WorktreePath: worktreePath,
			})
		}
		return ProjectDetailResourcesLoadedMsg{ProjectName: projectName, Resources: resources}
	}
}

// loadProjectPRsCmd returns a command that loads PRs asynchronously (phase 2: async data).
// Runs ListProjectPRs in a goroutine and returns ProjectPRsLoadedMsg with PRs grouped by repo.
func loadProjectPRsCmd(m *project.Manager, projectName string) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectPRsLoadedMsg{ProjectName: projectName, PRsByRepo: nil}
		}
		prsByRepo, _ := m.ListProjectPRs(projectName)
		return ProjectPRsLoadedMsg{ProjectName: projectName, PRsByRepo: prsByRepo}
	}
}

// loadResourceBeadsCmd returns a command that loads beads asynchronously (phase 3: async data).
// Spawns goroutines for each resource with a worktree, calls beads.ListForRepo/beads.ListForPR
// in parallel, and returns ResourceBeadsLoadedMsg with beads grouped by resource index.
func loadResourceBeadsCmd(projectName string, resources []project.Resource) tea.Cmd {
	return func() tea.Msg {
		beadsByResource := make(map[int][]project.BeadInfo)

		// Fetch beads concurrently across resources.
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i := range resources {
			r := &resources[i]
			if r.WorktreePath == "" {
				continue
			}
			wg.Add(1)
			go func(resIdx int) {
				defer wg.Done()
				var bdBeads []beads.Bead
				switch resources[resIdx].Kind {
				case project.ResourceRepo:
					bdBeads = beads.ListForRepo(resources[resIdx].WorktreePath, projectName)
				case project.ResourcePR:
					if resources[resIdx].PR != nil {
						bdBeads = beads.ListForPR(resources[resIdx].WorktreePath, projectName, resources[resIdx].PR.Number)
					}
				}
				beadInfos := make([]project.BeadInfo, len(bdBeads))
				for j, b := range bdBeads {
					beadInfos[j] = project.BeadInfo{
						ID:        b.ID,
						Title:     b.Title,
						Status:    b.Status,
						IssueType: b.IssueType,
						IsChild:   b.ParentID != "",
					}
				}
				mu.Lock()
				beadsByResource[resIdx] = beadInfos
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		return ResourceBeadsLoadedMsg{ProjectName: projectName, BeadsByResource: beadsByResource}
	}
}

// tickCmd returns a command that schedules a tickMsg after 5 seconds.
// Used for periodic refresh of panes and beads in project detail view.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// countBeadsFromResources counts open beads across the given resources.
// Used by loadProjectsCmd with resources from LoadProjectSummary to avoid
// a separate ListProjectResources call (which would redundantly fetch PRs).
// Bead counting is parallelized across resources for better performance.
func countBeadsFromResources(resources []project.Resource, projectName string) int {
	if len(resources) == 0 {
		return 0
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	totalCount := 0

	for _, r := range resources {
		if r.WorktreePath == "" {
			continue
		}
		wg.Add(1)
		go func(resource project.Resource) {
			defer wg.Done()
			var count int
			switch resource.Kind {
			case project.ResourceRepo:
				count = len(beads.ListForRepo(resource.WorktreePath, projectName))
			case project.ResourcePR:
				if resource.PR != nil {
					count = len(beads.ListForPR(resource.WorktreePath, projectName, resource.PR.Number))
				}
			}
			mu.Lock()
			totalCount += count
			mu.Unlock()
		}(r)
	}

	wg.Wait()
	return totalCount
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
