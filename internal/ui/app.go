package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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
	RalphStatus     *RalphStatusView // ralph status display
	ralphWorkdir    string            // workdir for current ralph run (for polling)
	termWidth       int               // terminal width from last WindowSizeMsg
	termHeight      int               // terminal height from last WindowSizeMsg
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
		// RalphStatus view removed - no longer handling window size updates for it
	}

	switch msg := msg.(type) {
	case RalphStatusMsg:
		// Ralph status polling removed - status file still written for CI/scripting
		// but UI no longer displays it (user sees output in tmux pane)
		return a, nil
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
	case ProjectsEnrichedMsg:
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
	case ProjectDetailResourcesLoadedMsg:
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
	case ProjectPRsLoadedMsg:
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
	case ProjectDetailPRsLoadedMsg:
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
	case ProjectDetailBeadsLoadedMsg:
		// Phase 3: Beads loaded, update view (complete)
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
			a.Detail.Resources = msg.Resources
			a.Detail.loadingPRs = false
			a.Detail.loadingBeads = false
			a.Detail.buildItems() // Rebuild list items to show beads
			a.refreshDetailPanes()
		}
		return a, nil
	case ResourceBeadsLoadedMsg:
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
				// Trigger progressive reload
				return a, tea.Batch(
					loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName),
				)
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
				// Trigger progressive reload
				return a, tea.Batch(
					loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName),
				)
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
			idx := a.Dashboard.Selected()
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
		// Refresh the resource list and pane info via progressive reload.
		if a.Mode == ModeProjectDetail && a.Detail != nil && a.Detail.ProjectName == msg.ProjectName {
			// Trigger progressive reload
			return a, loadProjectDetailResourcesCmd(a.ProjectManager, msg.ProjectName)
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
				prompt = fmt.Sprintf("Run `bd show %s` to understand the issue. Claim it with `bd update %s --status in_progress`, implement it, then close it with `bd close %s`. Follow the rules in .cursor/rules/.", selectedBead.ID, selectedBead.ID, selectedBead.ID)
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
		// Launch ralph binary with --project and --workdir flags.
		// If a specific bead is selected, add --bead flag (or --epic if it's an epic).
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Ralph: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		// Escape the workdir path for shell safety (handle spaces, special chars).
		escapedWorkdir := strings.ReplaceAll(workDir, "'", `'\''`)
		escapedProject := strings.ReplaceAll(a.Detail.ProjectName, "'", `'\''`)
		selectedBead := a.Detail.SelectedBead()
		cmd := fmt.Sprintf("%s --project '%s' --workdir '%s'", ralphPath, escapedProject, escapedWorkdir)
		if selectedBead != nil {
			escapedBead := strings.ReplaceAll(selectedBead.ID, "'", `'\''`)
			// If selected bead is an epic, use --epic flag for sequential leaf processing
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
		// Ralph status file (.ralph-status.json) is still written for CI/scripting,
		// but we don't poll it in the UI - user can see ralph output directly in tmux pane
		a.Status = "Ralph loop launched"
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
		detail, cmd := a.newProjectDetailView(msg.Name)
		a.Detail = detail
		return a, tea.Batch(a.Detail.Init(), cmd)
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
				return a, nil
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
	base := a.currentView().View()
	if a.KeyHandler != nil && a.KeyHandler.LeaderWaiting {
		base += "\n" + RenderKeybindHelp(a.KeyHandler, a.Mode)
	}
	if a.Overlays.Len() > 0 {
		if top, ok := a.Overlays.Peek(); ok {
			base += "\n" + top.View.View()
		}
	}
	// Ralph status view removed - user can see ralph output directly in tmux pane
	if a.Status != "" {
		style := Styles.Status
		if a.StatusIsError {
			style = Styles.TitleWarning
		}
		base += "\n" + style.Render("▶ "+a.Status) + Styles.Muted.Render(" (any key to dismiss)")
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
// Returns the view and a command to start progressive loading.
func (a *AppModel) newProjectDetailView(name string) (*ProjectDetailView, tea.Cmd) {
	v := NewProjectDetailView(name)
	if a.termWidth > 0 || a.termHeight > 0 {
		v.SetSize(a.termWidth, a.termHeight)
	}
	// Phase 1: Load repos instantly (filesystem-only, no GitHub API calls)
	var cmds []tea.Cmd
	if a.ProjectManager != nil {
		v.Resources = a.ProjectManager.ListProjectReposOnly(name)
		v.loadingPRs = true // PRs will be loaded asynchronously
		v.loadingBeads = false
		v.buildItems() // Build list items immediately so repos are visible
		cmds = append(cmds, v.spinnerTickCmd()) // Start spinner for PR loading
	}
	// Prune dead panes then populate pane info from session tracker
	if a.Sessions != nil {
		a.Sessions.Prune()
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

// loadProjectDetailPRsCmd returns a command that loads PRs asynchronously (phase 2: async data).
// Uses loadProjectPRsCmd to fetch PRs, then merges them into repo resources.
func loadProjectDetailPRsCmd(m *project.Manager, projectName string, repoResources []project.Resource) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectDetailPRsLoadedMsg{ProjectName: projectName, Resources: repoResources}
		}
		prsByRepo, _ := m.ListProjectPRs(projectName)
		projDir := m.ProjectDir(projectName)
		
		// Build map of PRs by repo name for quick lookup
		repoPRsMap := make(map[string][]project.PRInfo)
		for _, repoPRs := range prsByRepo {
			repoPRsMap[repoPRs.Repo] = repoPRs.PRs
		}
		
		// Build resources: repos + PRs in repo order.
		resources := make([]project.Resource, 0, len(repoResources))
		for _, repoRes := range repoResources {
			resources = append(resources, repoRes)
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
		
		return ProjectDetailPRsLoadedMsg{ProjectName: projectName, Resources: resources}
	}
}

// loadProjectDetailBeadsCmd returns a command that loads beads asynchronously (phase 3: async data).
// Beads are fetched in parallel across resources for optimal performance.
func loadProjectDetailBeadsCmd(projectName string, resourcesWithPRs []project.Resource) tea.Cmd {
	return func() tea.Msg {
		resources := make([]project.Resource, len(resourcesWithPRs))
		copy(resources, resourcesWithPRs)
		
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
				mu.Lock()
				resources[resIdx].Beads = make([]project.BeadInfo, len(bdBeads))
				for j, b := range bdBeads {
					resources[resIdx].Beads[j] = project.BeadInfo{
						ID:        b.ID,
						Title:     b.Title,
						Status:    b.Status,
						IssueType: b.IssueType,
						IsChild:   b.ParentID != "",
					}
				}
				mu.Unlock()
			}(i)
		}
		
		wg.Wait()
		
		return ProjectDetailBeadsLoadedMsg{ProjectName: projectName, Resources: resources}
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
