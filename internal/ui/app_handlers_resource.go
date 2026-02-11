package ui

import (
	"fmt"

	"devdeploy/internal/project"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

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
