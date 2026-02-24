package ui

import (
	"fmt"

	"devdeploy/internal/tmux"

	"devdeploy/internal/project"
	tea "github.com/charmbracelet/bubbletea"
)

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
