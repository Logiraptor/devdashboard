package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"devdeploy/internal/project"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

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
