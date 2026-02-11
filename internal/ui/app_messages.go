package ui

import (
	"devdeploy/internal/project"
	"time"
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
