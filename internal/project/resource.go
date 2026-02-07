package project

// ResourceKind distinguishes repo vs PR resources.
type ResourceKind string

const (
	// ResourceRepo is a git repo from ~/workspace.
	ResourceRepo ResourceKind = "repo"
	// ResourcePR is an open PR on a project repo.
	ResourcePR ResourceKind = "pr"
)

// PaneInfo tracks an active tmux pane associated with a resource.
// Stub for now; fleshed out in session tracker (devdeploy-7uj.3).
type PaneInfo struct {
	ID      string // tmux pane ID (e.g. "%42")
	IsAgent bool   // true if running `agent`, false for plain shell
}

// Resource unifies repos and PRs as first-class project items.
// The flat list is ordered repo-first, with PR resources immediately
// following their parent repo, enabling tree-style rendering.
type Resource struct {
	Kind         ResourceKind
	RepoName     string     // repo directory name in ~/workspace
	PR           *PRInfo    // non-nil for PR resources
	WorktreePath string     // populated when worktree exists; empty otherwise
	Panes        []PaneInfo // active tmux panes (from session tracker)
}
