package ui

import (
	"devdeploy/internal/project"
	"time"
)

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

// OpenCursorMsg opens Cursor IDE on the selected resource's worktree (SPC s c).
type OpenCursorMsg struct{}

// FocusPaneMsg focuses a pane by index (1-9) from the list of active panes.
type FocusPaneMsg struct {
	Index int // 1-based index into active panes list
}

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
// Kills associated panes and removes the worktree.
type RemoveResourceMsg struct {
	Resource project.Resource
}

// DismissModalMsg is sent when user cancels a modal (Esc).
type DismissModalMsg struct{}

// tickMsg triggers periodic refresh of panes and beads.
type tickMsg time.Time
