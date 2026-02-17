package worktree

// FindWorktreeForBranch finds the worktree that has the given branch checked out.
// Returns the worktree path, or empty string if not found.
// Searches from the source repository.
//
// If excludeSrcRepo is true, the source repository path itself is excluded
// from the search results (useful when you want to find a worktree other than
// the main repo).
//
// This function delegates to Manager.FindByBranch for unified worktree discovery.
// Consider using Manager.FindByBranch directly if you already have a Manager instance.
func FindWorktreeForBranch(srcRepo, branchName string, excludeSrcRepo bool) string {
	mgr, err := NewManager(srcRepo)
	if err != nil {
		return ""
	}
	return mgr.FindByBranch(branchName, excludeSrcRepo)
}
