package ui

import (
	"devdeploy/internal/project"
	"time"
)

// UpsertResourceSessionMsg creates/switches to the selected resource session.
type UpsertResourceSessionMsg struct{}

// KillResourceSessionMsg kills the selected resource session.
type KillResourceSessionMsg struct{}

// OpenCursorMsg opens Cursor IDE on the selected resource's worktree (SPC s c).
type OpenCursorMsg struct{}

// HomeRepoNamesLoadedMsg is phase 1 progressive loading data for home view.
// It includes repo names only (filesystem scan, instant).
type HomeRepoNamesLoadedMsg struct {
	Groups []project.RepoGroup
	Err    error
}

// HomeRepoGroupsLoadedMsg is phase 2 progressive loading data for home view.
// It includes repo headers + worktrees + PRs.
type HomeRepoGroupsLoadedMsg struct {
	Groups []project.RepoGroup
	Err    error
}

// HomeBeadsLoadedMsg is phase 3 progressive loading data for home view.
// It includes full repo groups with bead data attached.
type HomeBeadsLoadedMsg struct {
	Groups []project.RepoGroup
}

// ShowRemoveResourceMsg triggers the remove-resource confirmation.
type ShowRemoveResourceMsg struct{}

// RefreshMsg triggers a manual refresh: clears PR cache and reloads current view.
type RefreshMsg struct{}

// RefreshBeadsMsg triggers a refresh of beads for all resources in home view.
type RefreshBeadsMsg struct{}

// CloseBeadMsg triggers closing the currently selected bead.
type CloseBeadMsg struct{}

// RemoveResourceMsg is sent when user confirms removal of a resource.
// Kills associated session and removes the worktree.
type RemoveResourceMsg struct {
	Resource project.Resource
}

// ShowAddWorktreeMsg triggers the add-worktree text input modal.
type ShowAddWorktreeMsg struct{}

// AddWorktreeMsg is sent when user confirms a new worktree branch name.
type AddWorktreeMsg struct {
	RepoName   string
	BranchName string
}

// DismissModalMsg is sent when user cancels a modal (Esc).
type DismissModalMsg struct{}

// tickMsg triggers periodic refresh of sessions, preview, and beads.
type tickMsg time.Time

// homeTickRefreshedMsg carries async tick refresh results back into Update.
type homeTickRefreshedMsg struct {
	PruneErr    error
	PreviewText string
	PreviewErr  error
	HasPreview  bool
}
