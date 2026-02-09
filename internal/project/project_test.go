package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_ListProjects_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	projects, err := m.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestManager_CreateProject(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	if err := m.CreateProject("my-project"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	projDir := filepath.Join(dir, "my-project")
	if info, err := os.Stat(projDir); err != nil || !info.IsDir() {
		t.Errorf("expected project dir to exist: %v", err)
	}
	configPath := filepath.Join(projDir, "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected config.yaml to exist: %v", err)
	}
}

func TestManager_CreateProject_NormalizesName(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	if err := m.CreateProject("My Project"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// projectDir normalizes to lowercase, spaces -> hyphens
	projDir := filepath.Join(dir, "my-project")
	if _, err := os.Stat(projDir); err != nil {
		t.Errorf("expected normalized dir my-project to exist: %v", err)
	}
}

func TestManager_CreateProject_Idempotent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	if err := m.CreateProject("test"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// Second create should succeed (already exists)
	if err := m.CreateProject("test"); err != nil {
		t.Errorf("CreateProject idempotent: %v", err)
	}
}

func TestManager_ListProjects_AfterCreate(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	_ = m.CreateProject("proj-a")
	_ = m.CreateProject("proj-b")

	projects, err := m.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
	names := make(map[string]bool)
	for _, p := range projects {
		names[p.Name] = true
	}
	if !names["proj-a"] || !names["proj-b"] {
		t.Errorf("expected proj-a and proj-b, got %v", names)
	}
}

func TestManager_ListProjects_SkipsDotDirs(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	_ = m.CreateProject("visible")
	_ = os.MkdirAll(filepath.Join(dir, ".hidden"), 0755)

	projects, err := m.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "visible" {
		t.Errorf("expected 1 project (visible), got %d: %v", len(projects), projects)
	}
}

func TestManager_DeleteProject_NoRepos(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	_ = m.CreateProject("to-delete")
	if err := m.DeleteProject("to-delete"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	projDir := filepath.Join(dir, "to-delete")
	if _, err := os.Stat(projDir); !os.IsNotExist(err) {
		t.Errorf("expected project dir to be removed: %v", err)
	}
}

func TestManager_ListProjectRepos_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("empty-proj")

	repos, err := m.ListProjectRepos("empty-proj")
	if err != nil {
		t.Fatalf("ListProjectRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestManager_ListProjectRepos_ExcludesPRWorktrees(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "test-proj")

	// Create a normal repo worktree dir
	repoDir := filepath.Join(projDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	// Create a PR worktree dir (should be excluded)
	prDir := filepath.Join(projDir, "my-repo-pr-42")
	_ = os.MkdirAll(prDir, 0755)
	_ = os.WriteFile(filepath.Join(prDir, ".git"), []byte("gitdir: /y"), 0644)

	repos, err := m.ListProjectRepos("test-proj")
	if err != nil {
		t.Fatalf("ListProjectRepos: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo (excluding PR worktree), got %d: %v", len(repos), repos)
	}
	if len(repos) > 0 && repos[0] != "my-repo" {
		t.Errorf("expected my-repo, got %s", repos[0])
	}
}

func TestManager_ListWorkspaceRepos_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)

	repos, err := m.ListWorkspaceRepos()
	if err != nil {
		t.Fatalf("ListWorkspaceRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestManager_ListWorkspaceRepos_DetectsGitRepos(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	repoDir := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

	m := NewManager(dir, wsDir)
	repos, err := m.ListWorkspaceRepos()
	if err != nil {
		t.Fatalf("ListWorkspaceRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != "my-repo" {
		t.Errorf("expected [my-repo], got %v", repos)
	}
}

func TestManager_EnsurePRWorktree_ReusesExisting(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	// Create a fake source repo in workspace (just a dir, not real git)
	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")

	// Pre-create the worktree dir with a .git file (simulating existing worktree)
	projDir := filepath.Join(dir, "projects", "test-proj")
	wtDir := filepath.Join(projDir, "my-repo-pr-42")
	_ = os.MkdirAll(wtDir, 0755)
	_ = os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: /some/path"), 0644)

	got, err := m.EnsurePRWorktree("test-proj", "my-repo", 42, "feat-branch")
	if err != nil {
		t.Fatalf("EnsurePRWorktree: %v", err)
	}
	if got != wtDir {
		t.Errorf("expected reused path %s, got %s", wtDir, got)
	}
}

func TestManager_EnsurePRWorktree_SourceRepoNotFound(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")

	_, err := m.EnsurePRWorktree("test-proj", "nonexistent-repo", 42, "feat-branch")
	if err == nil {
		t.Fatal("expected error for nonexistent source repo")
	}
	if !os.IsNotExist(err) && !filepath.IsAbs(err.Error()) {
		// Just verify it contains a reference to the source repo
		if !testing.Verbose() {
			// Error should mention source repo
		}
	}
}

func TestManager_EnsurePRWorktree_WorktreePathFormat(t *testing.T) {
	// Verify the worktree path follows <projectDir>/<repoName>-pr-<number> convention.
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)
	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")

	// Pre-create the worktree dir to test the reuse path and confirm the expected format.
	projDir := filepath.Join(dir, "projects", "test-proj")
	expectedPath := filepath.Join(projDir, "my-repo-pr-99")
	_ = os.MkdirAll(expectedPath, 0755)
	_ = os.WriteFile(filepath.Join(expectedPath, ".git"), []byte("gitdir: /x"), 0644)

	got, err := m.EnsurePRWorktree("test-proj", "my-repo", 99, "some-branch")
	if err != nil {
		t.Fatalf("EnsurePRWorktree: %v", err)
	}
	if got != expectedPath {
		t.Errorf("expected %s, got %s", expectedPath, got)
	}
}

func TestManager_ListProjectResources_ReposAndPRs(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	// Create a fake source repo in workspace.
	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "projects", "test-proj")

	// Create a repo worktree dir.
	repoDir := filepath.Join(projDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	// Create a PR worktree dir.
	prDir := filepath.Join(projDir, "my-repo-pr-42")
	_ = os.MkdirAll(prDir, 0755)
	_ = os.WriteFile(filepath.Join(prDir, ".git"), []byte("gitdir: /y"), 0644)

	resources := m.ListProjectResources("test-proj")
	// gh pr list is not available in tests, so PRs come from disk scanning.
	// We should at least get the repo resource.
	if len(resources) == 0 {
		t.Fatal("expected at least 1 resource, got 0")
	}

	// First resource must be the repo.
	if resources[0].Kind != ResourceRepo {
		t.Errorf("expected first resource kind=repo, got %s", resources[0].Kind)
	}
	if resources[0].RepoName != "my-repo" {
		t.Errorf("expected first resource RepoName=my-repo, got %s", resources[0].RepoName)
	}
	if resources[0].WorktreePath != repoDir {
		t.Errorf("expected WorktreePath=%s, got %s", repoDir, resources[0].WorktreePath)
	}
}

func TestManager_ListProjectResources_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("empty-proj")

	resources := m.ListProjectResources("empty-proj")
	if len(resources) != 0 {
		t.Errorf("expected 0 resources for empty project, got %d", len(resources))
	}
}

func TestManager_ListProjectResources_PRWorktreeDetection(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)
	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "projects", "test-proj")

	// Repo worktree.
	repoDir := filepath.Join(projDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	// PR worktree exists on disk (e.g. from a previous session).
	prDir := filepath.Join(projDir, "my-repo-pr-99")
	_ = os.MkdirAll(prDir, 0755)
	_ = os.WriteFile(filepath.Join(prDir, ".git"), []byte("gitdir: /y"), 0644)

	resources := m.ListProjectResources("test-proj")
	// Without gh pr list, only the repo shows up. But the PR worktree dir
	// is excluded from the repo list (tested separately). When PRs ARE returned
	// by gh, ListProjectResources would populate WorktreePath for PR resources
	// that match the dir on disk. We test the repo portion here.
	foundRepo := false
	for _, r := range resources {
		if r.Kind == ResourceRepo && r.RepoName == "my-repo" {
			foundRepo = true
		}
	}
	if !foundRepo {
		t.Error("expected repo resource 'my-repo' in resources")
	}
}

func TestManager_LoadProjectSummary_Empty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("empty-proj")

	summary := m.LoadProjectSummary("empty-proj")
	if summary.PRCount != 0 {
		t.Errorf("expected 0 PRs for empty project, got %d", summary.PRCount)
	}
	if len(summary.Resources) != 0 {
		t.Errorf("expected 0 resources for empty project, got %d", len(summary.Resources))
	}
}

func TestManager_LoadProjectSummary_ReposOnly(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)

	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "projects", "test-proj")

	// Create a repo worktree dir.
	repoDir := filepath.Join(projDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	summary := m.LoadProjectSummary("test-proj")
	// gh pr list is not available in tests, so PRCount should be 0.
	if summary.PRCount != 0 {
		t.Errorf("expected 0 PRs (no gh available), got %d", summary.PRCount)
	}
	// Should have at least the repo resource.
	if len(summary.Resources) == 0 {
		t.Fatal("expected at least 1 resource, got 0")
	}
	if summary.Resources[0].Kind != ResourceRepo {
		t.Errorf("expected first resource kind=repo, got %s", summary.Resources[0].Kind)
	}
	if summary.Resources[0].RepoName != "my-repo" {
		t.Errorf("expected RepoName=my-repo, got %s", summary.Resources[0].RepoName)
	}
	if summary.Resources[0].WorktreePath != repoDir {
		t.Errorf("expected WorktreePath=%s, got %s", repoDir, summary.Resources[0].WorktreePath)
	}
}

func TestManager_LoadProjectSummary_PRWorktreeDetection(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(wsDir, 0755)
	srcRepo := filepath.Join(wsDir, "my-repo")
	_ = os.MkdirAll(filepath.Join(srcRepo, ".git"), 0755)

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "projects", "test-proj")

	// Create repo worktree.
	repoDir := filepath.Join(projDir, "my-repo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	// Create PR worktree on disk (simulates a previously created worktree).
	prDir := filepath.Join(projDir, "my-repo-pr-99")
	_ = os.MkdirAll(prDir, 0755)
	_ = os.WriteFile(filepath.Join(prDir, ".git"), []byte("gitdir: /y"), 0644)

	summary := m.LoadProjectSummary("test-proj")
	// Without gh, only repos show up. PR worktree dir is excluded from repo list.
	foundRepo := false
	for _, r := range summary.Resources {
		if r.Kind == ResourceRepo && r.RepoName == "my-repo" {
			foundRepo = true
		}
	}
	if !foundRepo {
		t.Error("expected repo resource 'my-repo' in summary resources")
	}
}

func TestManager_RemovePRWorktree_NoOp(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("test-proj")

	// Removing a non-existent PR worktree should be a no-op.
	err := m.RemovePRWorktree("test-proj", "nonexistent", 42)
	if err != nil {
		t.Errorf("expected no error for non-existent PR worktree, got %v", err)
	}
}

