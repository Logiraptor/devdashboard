package ui

import (
	"fmt"

	"devdeploy/internal/bd"

	tea "github.com/charmbracelet/bubbletea"
)

func (a *appModelAdapter) handleHomeRepoNamesLoaded(msg HomeRepoNamesLoadedMsg) (tea.Model, tea.Cmd) {
	if a.Home == nil {
		a.Home = NewHomeView()
		a.Home.getGlobalSessions = a.getGlobalSessionsForDisplay
	}
	if msg.Err != nil {
		a.Status = fmt.Sprintf("Load repos: %v", msg.Err)
		a.StatusIsError = true
		a.Home.loadingGroups = false
		a.Home.loadingBeads = false
		return a, nil
	}
	a.Home.SetRepoGroups(msg.Groups)
	a.Home.loadingGroups = true
	a.Home.loadingBeads = false
	return a, tea.Batch(
		a.Home.spinnerTickCmd(),
		loadHomeRepoGroupsCmd(a.ProjectManager),
	)
}

func (a *appModelAdapter) handleHomeRepoGroupsLoaded(msg HomeRepoGroupsLoadedMsg) (tea.Model, tea.Cmd) {
	if a.Home == nil {
		a.Home = NewHomeView()
		a.Home.getGlobalSessions = a.getGlobalSessionsForDisplay
	}
	if msg.Err != nil {
		a.Status = fmt.Sprintf("Load repo groups: %v", msg.Err)
		a.StatusIsError = true
		a.Home.loadingGroups = false
		a.Home.loadingBeads = false
		return a, nil
	}
	a.Home.SetRepoGroups(msg.Groups)
	a.Home.loadingGroups = false
	a.Home.loadingBeads = true
	a.refreshHomeSessions()
	if len(msg.Groups) == 0 {
		a.Home.loadingBeads = false
		return a, nil
	}
	return a, tea.Batch(a.Home.spinnerTickCmd(), loadHomeBeadsCmd(msg.Groups))
}

func (a *appModelAdapter) handleHomeBeadsLoaded(msg HomeBeadsLoadedMsg) (tea.Model, tea.Cmd) {
	if a.Home != nil {
		a.Home.loadingGroups = false
		a.Home.loadingBeads = false
		a.Home.SetRepoGroups(msg.Groups)
		a.refreshHomeSessions()
	}
	return a, nil
}

// handleRefreshBeads handles RefreshBeadsMsg by refreshing beads for all home resources.
func (a *appModelAdapter) handleRefreshBeads() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}
	groups := a.Home.RepoGroups()
	if len(groups) == 0 {
		return a, nil
	}
	a.Home.loadingBeads = true
	a.Home.buildItems()
	a.refreshHomeSessions()
	return a, tea.Batch(a.Home.spinnerTickCmd(), loadHomeBeadsCmd(groups))
}

// handleCloseBead handles CloseBeadMsg by closing the currently selected bead.
func (a *appModelAdapter) handleCloseBead() (tea.Model, tea.Cmd) {
	if a.Home == nil {
		return a, nil
	}

	bead := a.Home.SelectedBead()
	if bead == nil {
		a.Status = "No bead selected (cursor on resource header)"
		a.StatusIsError = true
		return a, nil
	}

	resource := a.Home.SelectedResource()
	if resource == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}

	worktreePath := resource.WorktreePath
	if worktreePath == "" {
		a.Status = fmt.Sprintf("No worktree for resource %s", resource.RepoName)
		a.StatusIsError = true
		return a, nil
	}

	_, err := bd.Run(worktreePath, "close", bead.ID)
	if err != nil {
		a.Status = fmt.Sprintf("Close bead %s: %v", bead.ID, err)
		a.StatusIsError = true
		return a, nil
	}

	a.Status = fmt.Sprintf("Closed bead %s", bead.ID)
	a.StatusIsError = false
	return a, func() tea.Msg { return RefreshBeadsMsg{} }
}
