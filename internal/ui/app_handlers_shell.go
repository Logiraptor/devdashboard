package ui

import (
	"fmt"
	"os"
	"os/exec"

	"devdeploy/internal/progress"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

// handleUpsertResourceSession handles Enter by creating/switching to a resource session.
func (a *appModelAdapter) handleUpsertResourceSession() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	r := a.Home.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	workDir, err := a.ensureResourceWorktree(r)
	if err != nil {
		a.Status = fmt.Sprintf("Open resource session: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	rk := resourceKeyFromResource(*r)
	sessionName := rk.SessionName()
	created, err := tmux.EnsureSession(sessionName, workDir, os.Getenv("SHELL"))
	if err != nil {
		a.Status = fmt.Sprintf("Open resource session: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		a.Sessions.Register(rk, sessionName)
		a.refreshHomeSessions()
	}
	if err := tmux.SwitchClient(sessionName); err != nil {
		a.Status = fmt.Sprintf("Switch to session: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if created {
		a.Status = fmt.Sprintf("Created session %s", sessionName)
	} else {
		a.Status = fmt.Sprintf("Switched to session %s", sessionName)
	}
	a.StatusIsError = false
	return a, nil
}

// handleKillResourceSession handles SPC s k by closing the selected resource session.
func (a *appModelAdapter) handleKillResourceSession() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	r := a.Home.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}

	rk := resourceKeyFromResource(*r)
	sessionName := rk.SessionName()
	exists, err := tmux.SessionExists(sessionName)
	if err != nil {
		a.Status = fmt.Sprintf("Kill session: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if !exists {
		if a.Sessions != nil {
			a.Sessions.UnregisterByName(sessionName)
			a.refreshHomeSessions()
		}
		a.Status = fmt.Sprintf("Session %s is already closed", sessionName)
		a.StatusIsError = false
		return a, nil
	}
	if err := tmux.KillSession(sessionName); err != nil {
		a.Status = fmt.Sprintf("Kill session: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		a.Sessions.UnregisterByName(sessionName)
		a.refreshHomeSessions()
	}
	a.Status = fmt.Sprintf("Killed session %s", sessionName)
	a.StatusIsError = false
	return a, nil
}

// handleOpenCursor opens Cursor IDE on the selected resource's worktree.
func (a *appModelAdapter) handleOpenCursor() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	r := a.Home.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	workDir, err := a.ensureResourceWorktree(r)
	if err != nil {
		a.Status = fmt.Sprintf("Open Cursor: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	cmd := exec.Command("cursor", workDir)
	if err := cmd.Start(); err != nil {
		a.Status = fmt.Sprintf("Open Cursor: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	go func() { _ = cmd.Wait() }()
	a.Status = fmt.Sprintf("Opened Cursor in %s", workDir)
	a.StatusIsError = false
	return a, nil
}

// handleTick handles tickMsg by refreshing sessions, preview, and beads.
func (a *appModelAdapter) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	_ = msg
	if a.Home == nil {
		return a, tickCmd()
	}
	cmds := []tea.Cmd{
		tickCmd(),
		refreshHomeTickCmd(a.Sessions, a.selectedResourceSessionName()),
	}
	if groups := a.Home.RepoGroups(); a.ProjectManager != nil && len(groups) > 0 && !a.Home.loadingBeads {
		a.Home.loadingBeads = true
		a.Home.buildItems()
		cmds = append(cmds, loadHomeBeadsCmd(groups))
	}
	return a, tea.Batch(cmds...)
}

// handleHomeTickRefreshed applies asynchronous tick refresh results.
func (a *appModelAdapter) handleHomeTickRefreshed(msg homeTickRefreshedMsg) (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	a.populateHomeSessions(a.Home)
	if !msg.HasPreview {
		a.Home.SetPreview("", false)
		return a, nil
	}
	if msg.PreviewErr != nil {
		a.Home.SetPreview(fmt.Sprintf("capture error: %v", msg.PreviewErr), true)
		return a, nil
	}
	a.Home.SetPreview(msg.PreviewText, true)
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
	if a.ProjectManager != nil {
		a.ProjectManager.ClearPRCache()
	}
	if a.Home != nil {
		a.Home.loadingGroups = true
		a.Home.loadingBeads = false
	}
	if a.ProjectManager != nil {
		return a, loadHomeRepoGroupsCmd(a.ProjectManager)
	}
	return a, nil
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
