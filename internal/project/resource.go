package project

// ResourceKind distinguishes repo, worktree, and PR resources.
type ResourceKind string

const (
	// ResourceRepo is a git repo from ~/workspace.
	ResourceRepo ResourceKind = "repo"
	// ResourceWorktree is a non-main git worktree attached to a repo.
	ResourceWorktree ResourceKind = "worktree"
	// ResourcePR is an open PR on a project repo.
	ResourcePR ResourceKind = "pr"
)

// SessionInfo tracks an active tmux session associated with a resource.
// Populated from session.Tracker during view construction.
type SessionInfo struct {
	Name string // tmux session name (e.g. "dd-repo-devdeploy")
}

// BeadInfo holds a bd issue associated with a resource for display.
type BeadInfo struct {
	ID          string   // bead identifier (e.g. "devdeploy-abc")
	Title       string   // short summary
	Description string   // full description text
	Status      string   // "open", "in_progress", etc.
	IssueType   string   // "epic", "task", "bug", etc.
	Labels      []string // issue labels
	IsChild     bool     // true if this bead is a child of an epic
}

// WorktreeInfo describes a non-main worktree attached to a repo.
type WorktreeInfo struct {
	Branch string // checked-out branch name
	Path   string // absolute filesystem path for the worktree
}

// Resource unifies repos, worktrees, and PRs as first-class project items.
// Grouped resources are rendered repo-first, with related items nested beneath.
type Resource struct {
	Kind         ResourceKind
	RepoName     string        // repo directory name in ~/workspace
	PR           *PRInfo       // non-nil for PR resources
	Worktree     *WorktreeInfo // non-nil for worktree resources
	WorktreePath string        // populated when worktree exists; empty otherwise
	Session      *SessionInfo  // active tmux session (from session tracker)
	Beads        []BeadInfo    // bd issues associated with this resource
}

// RepoGroup represents a workspace repo and all related resources.
type RepoGroup struct {
	RepoName      string
	WorkspacePath string
	Items         []Resource
}
