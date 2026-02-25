package ui

import (
	"fmt"

	"devdeploy/internal/tmux"

	"devdeploy/internal/project"
	tea "github.com/charmbracelet/bubbletea"
)

// handleRemoveResource handles RemoveResourceMsg by killing sessions and removing worktrees.
func (a *appModelAdapter) handleRemoveResource(msg RemoveResourceMsg) (tea.Model, tea.Cmd) {
	if a.ProjectManager == nil {
		return a, nil
	}
	// Kill associated tmux session (best-effort; session may already be dead).
	if a.Sessions != nil {
		rk := resourceKeyFromResource(msg.Resource)
		if tracked, ok := a.Sessions.SessionForResource(rk); ok {
			_ = tmux.KillSession(tracked.Name) // ignore errors for dead sessions
		}
		a.Sessions.Unregister(rk)
	}
	// Remove worktree based on resource kind.
	var removeErr error
	switch msg.Resource.Kind {
	case project.ResourceWorktree, project.ResourcePR:
		if msg.Resource.WorktreePath != "" {
			removeErr = a.ProjectManager.RemoveWorktreePath(msg.Resource.RepoName, msg.Resource.WorktreePath)
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
		if msg.Resource.Kind == project.ResourceWorktree {
			branch := ""
			if msg.Resource.Worktree != nil {
				branch = msg.Resource.Worktree.Branch
			}
			if branch != "" {
				label = fmt.Sprintf("worktree %s@%s", msg.Resource.RepoName, branch)
			} else {
				label = fmt.Sprintf("worktree %s", msg.Resource.RepoName)
			}
		}
		a.Status = fmt.Sprintf("Removed %s", label)
		a.StatusIsError = false
	}
	a.Overlays.Pop()
	// Refresh home data after removal.
	return a, loadHomeRepoGroupsCmd(a.ProjectManager)
}

// handleShowAddWorktree handles ShowAddWorktreeMsg by showing a text input modal for the branch name.
func (a *appModelAdapter) handleShowAddWorktree() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	r := a.Home.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	if r.Kind != project.ResourceRepo {
		a.Status = "Select a repo header to add a worktree"
		a.StatusIsError = true
		return a, nil
	}
	repoName := r.RepoName
	modal := NewTextInputModal(
		"Add worktree to "+repoName,
		"branch-name",
		func(value string) tea.Msg {
			return AddWorktreeMsg{RepoName: repoName, BranchName: value}
		},
	)
	a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
	return a, modal.Init()
}

// handleAddWorktree handles AddWorktreeMsg by creating the worktree.
func (a *appModelAdapter) handleAddWorktree(msg AddWorktreeMsg) (tea.Model, tea.Cmd) {
	if a.ProjectManager == nil {
		a.Status = "No project manager available"
		a.StatusIsError = true
		return a, nil
	}
	a.Overlays.Pop()

	_, err := a.ProjectManager.CreateWorktree(msg.RepoName, msg.BranchName)
	if err != nil {
		a.Status = fmt.Sprintf("Add worktree: %v", err)
		a.StatusIsError = true
		return a, nil
	}

	a.Status = fmt.Sprintf("Created worktree %s@%s", msg.RepoName, msg.BranchName)
	a.StatusIsError = false
	return a, loadHomeRepoGroupsCmd(a.ProjectManager)
}

// handleShowRemoveResource handles ShowRemoveResourceMsg by showing the remove resource confirmation modal.
func (a *appModelAdapter) handleShowRemoveResource() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	r := a.Home.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	if r.Kind == project.ResourceRepo {
		a.Status = "Cannot remove primary repo checkout"
		a.StatusIsError = true
		return a, nil
	}
	modal := NewRemoveResourceConfirmModal(*r)
	a.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
	return a, modal.Init()
}
