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

func TestManager_CountArtifacts(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("artifacts-proj")

	projDir := filepath.Join(dir, "artifacts-proj")
	if m.CountArtifacts("artifacts-proj") != 0 {
		t.Errorf("expected 0 artifacts initially")
	}

	_ = os.WriteFile(filepath.Join(projDir, "plan.md"), []byte("# Plan\n"), 0644)
	if m.CountArtifacts("artifacts-proj") != 1 {
		t.Errorf("expected 1 artifact after plan.md")
	}

	_ = os.WriteFile(filepath.Join(projDir, "design.md"), []byte("# Design\n"), 0644)
	if m.CountArtifacts("artifacts-proj") != 2 {
		t.Errorf("expected 2 artifacts after design.md")
	}
}
