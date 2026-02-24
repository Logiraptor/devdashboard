package project

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestManager_ListProjectReposOnly_ReturnsRepoResources(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, dir)
	_ = m.CreateProject("test-proj")
	projDir := filepath.Join(dir, "test-proj")

	// Create repo worktree dirs
	repo1Dir := filepath.Join(projDir, "repo-a")
	_ = os.MkdirAll(repo1Dir, 0755)
	_ = os.WriteFile(filepath.Join(repo1Dir, ".git"), []byte("gitdir: /x"), 0644)

	repo2Dir := filepath.Join(projDir, "repo-b")
	_ = os.MkdirAll(repo2Dir, 0755)
	_ = os.WriteFile(filepath.Join(repo2Dir, ".git"), []byte("gitdir: /y"), 0644)

	// Create a PR worktree dir (should be excluded)
	prDir := filepath.Join(projDir, "repo-a-pr-42")
	_ = os.MkdirAll(prDir, 0755)
	_ = os.WriteFile(filepath.Join(prDir, ".git"), []byte("gitdir: /z"), 0644)

	resources := m.ListProjectReposOnly("test-proj")
	if len(resources) != 2 {
		t.Errorf("expected 2 repo resources, got %d", len(resources))
	}

	// Verify all resources are repo-type
	for i, r := range resources {
		if r.Kind != ResourceRepo {
			t.Errorf("resource[%d]: expected ResourceRepo, got %v", i, r.Kind)
		}
		if r.PR != nil {
			t.Errorf("resource[%d]: expected nil PR, got %v", i, r.PR)
		}
	}

	// Verify repo names
	repoNames := make(map[string]bool)
	for _, r := range resources {
		repoNames[r.RepoName] = true
		if r.WorktreePath != filepath.Join(projDir, r.RepoName) {
			t.Errorf("resource %s: expected WorktreePath %s, got %s", r.RepoName, filepath.Join(projDir, r.RepoName), r.WorktreePath)
		}
	}
	if !repoNames["repo-a"] || !repoNames["repo-b"] {
		t.Errorf("expected repo-a and repo-b, got %v", repoNames)
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

func TestManager_ListRepoGroups_IncludesWorktreesAndPRs(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	repoPath := filepath.Join(wsDir, "repo-a")
	wtPath := filepath.Join(dir, "wt", "repo-a-feature")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755), "create repo .git dir")
	require.NoError(t, os.MkdirAll(wtPath, 0755), "create worktree dir")

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755), "create bin dir")

	gitScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-C" ]; then
	repo="$2"
	shift 2
fi
if [ "$1" = "worktree" ] && [ "$2" = "list" ]; then
	cat <<EOF
worktree %s
HEAD 1111111
branch refs/heads/main

worktree %s
HEAD 2222222
branch refs/heads/feature/one
EOF
	exit 0
fi
echo "unexpected git args: $*" >&2
exit 1
`, repoPath, wtPath)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"), []byte(gitScript), 0755), "write fake git")

	ghScript := `#!/bin/sh
if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
	echo "my-org"
	exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
	search=0
	for arg in "$@"; do
		case "$arg" in
			team-review-requested:*)
				search=1
				;;
		esac
	done
	if [ "$search" -eq 1 ]; then
		echo '[{"number":2,"title":"Team PR","state":"OPEN","headRefName":"feature/two","mergedAt":null}]'
	else
		echo '[{"number":1,"title":"My PR","state":"OPEN","headRefName":"feature/one","mergedAt":null}]'
	fi
	exit 0
fi
echo "unexpected gh args: $*" >&2
exit 1
`
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "gh"), []byte(ghScript), 0755), "write fake gh")

	originalPath := os.Getenv("PATH")
	require.NoError(t, os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath), "set PATH")
	defer func() {
		_ = os.Setenv("PATH", originalPath)
	}()

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	groups, err := m.ListRepoGroups()
	require.NoError(t, err)
	require.Len(t, groups, 1, "expected 1 repo group")

	group := groups[0]
	require.Equal(t, "repo-a", group.RepoName, "repo group name")
	require.Equal(t, repoPath, group.WorkspacePath, "workspace path")
	require.Len(t, group.Items, 4, "expected 4 resources (repo + worktree + 2 PRs)")

	require.Equal(t, ResourceRepo, group.Items[0].Kind, "first item should be repo")
	require.Equal(t, ResourceWorktree, group.Items[1].Kind, "second item should be worktree")
	require.NotNil(t, group.Items[1].Worktree, "worktree metadata should be set")
	require.Equal(t, "feature/one", group.Items[1].Worktree.Branch, "worktree branch")
	require.Equal(t, ResourcePR, group.Items[2].Kind, "third item should be PR")
	require.Equal(t, ResourcePR, group.Items[3].Kind, "fourth item should be PR")
	require.Equal(t, wtPath, group.Items[2].WorktreePath, "PR on feature/one should match worktree path")
	require.Empty(t, group.Items[3].WorktreePath, "unmatched PR should not have a worktree path")
}

func TestManager_ListRepoGroups_UsesPRCache(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	repoPath := filepath.Join(wsDir, "repo-a")
	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, ".git"), 0755), "create repo .git dir")

	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755), "create bin dir")
	counterPath := filepath.Join(dir, "gh-pr-count")

	gitScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-C" ]; then
	shift 2
fi
if [ "$1" = "worktree" ] && [ "$2" = "list" ]; then
	cat <<EOF
worktree %s
HEAD 1111111
branch refs/heads/main
EOF
	exit 0
fi
echo "unexpected git args: $*" >&2
exit 1
`, repoPath)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "git"), []byte(gitScript), 0755), "write fake git")

	ghScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
	echo "my-org"
	exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
	count=0
	if [ -f %q ]; then
		count=$(cat %q)
	fi
	count=$((count + 1))
	echo "$count" > %q
	echo '[{"number":1,"title":"My PR","state":"OPEN","headRefName":"feature/one","mergedAt":null}]'
	exit 0
fi
echo "unexpected gh args: $*" >&2
exit 1
`, counterPath, counterPath, counterPath)
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "gh"), []byte(ghScript), 0755), "write fake gh")

	originalPath := os.Getenv("PATH")
	require.NoError(t, os.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath), "set PATH")
	defer func() {
		_ = os.Setenv("PATH", originalPath)
	}()

	m := NewManager(filepath.Join(dir, "projects"), wsDir)
	first, err := m.ListRepoGroups()
	require.NoError(t, err)
	second, err := m.ListRepoGroups()
	require.NoError(t, err)
	require.Len(t, first, 1, "first call should return one repo group")
	require.Len(t, second, 1, "second call should return one repo group")

	content, err := os.ReadFile(counterPath)
	require.NoError(t, err, "read gh counter")
	require.Equal(t, "2\n", string(content), "expected exactly 2 gh pr list calls (first run only)")
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
	// Just verify we got an error - the exact error type/message depends on
	// how far the function gets before failing with a nonexistent repo.
	_ = os.IsNotExist(err) // suppress unused warning; we just need the error check above
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

// --- Implicit project tests ---

func setupWorkspaceRepo(t *testing.T, wsDir, repoName string) string {
	t.Helper()
	repoPath := filepath.Join(wsDir, repoName)
	_ = os.MkdirAll(filepath.Join(repoPath, ".git"), 0755)
	_ = os.WriteFile(filepath.Join(repoPath, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
	return repoPath
}

func TestManager_IsImplicitProject(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	setupWorkspaceRepo(t, wsDir, "my-repo")
	_ = m.CreateProject("real-proj")

	if !m.IsImplicitProject("my-repo") {
		t.Error("workspace repo without a project should be implicit")
	}
	if m.IsImplicitProject("real-proj") {
		t.Error("real project should not be implicit")
	}
	if m.IsImplicitProject("nonexistent") {
		t.Error("nonexistent name should not be implicit")
	}
}

func TestManager_IsImplicitProject_RealProjectOverrides(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	setupWorkspaceRepo(t, wsDir, "overlap")
	_ = m.CreateProject("overlap")

	if m.IsImplicitProject("overlap") {
		t.Error("repo with a same-named real project should not be implicit")
	}
}

func TestManager_ListProjects_IncludesImplicit(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	_ = m.CreateProject("real-proj")
	setupWorkspaceRepo(t, wsDir, "my-repo")

	projects, err := m.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects (1 real + 1 implicit), got %d", len(projects))
	}

	byName := map[string]ProjectInfo{}
	for _, p := range projects {
		byName[p.Name] = p
	}

	real := byName["real-proj"]
	if real.Immutable {
		t.Error("real project should not be immutable")
	}

	implicit := byName["my-repo"]
	if !implicit.Immutable {
		t.Error("implicit project should be immutable")
	}
	if implicit.RepoCount != 1 {
		t.Errorf("implicit project should have RepoCount=1, got %d", implicit.RepoCount)
	}
	if implicit.Dir != filepath.Join(wsDir, "my-repo") {
		t.Errorf("implicit project Dir: want %s, got %s", filepath.Join(wsDir, "my-repo"), implicit.Dir)
	}
}

func TestManager_ListProjects_ImplicitSuppressedByRealProject(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	setupWorkspaceRepo(t, wsDir, "overlap")
	_ = m.CreateProject("overlap")

	projects, err := m.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project (real shadows implicit), got %d", len(projects))
	}
	if projects[0].Immutable {
		t.Error("should be the real project, not the implicit one")
	}
}

func TestManager_ListProjectRepos_Implicit(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	setupWorkspaceRepo(t, wsDir, "my-repo")

	repos, err := m.ListProjectRepos("my-repo")
	if err != nil {
		t.Fatalf("ListProjectRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != "my-repo" {
		t.Errorf("expected [my-repo], got %v", repos)
	}
}

func TestManager_ListProjectReposOnly_Implicit(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	repoPath := setupWorkspaceRepo(t, wsDir, "my-repo")

	resources := m.ListProjectReposOnly("my-repo")
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r.Kind != ResourceRepo {
		t.Errorf("expected ResourceRepo, got %s", r.Kind)
	}
	if r.RepoName != "my-repo" {
		t.Errorf("expected RepoName=my-repo, got %s", r.RepoName)
	}
	if r.WorktreePath != repoPath {
		t.Errorf("expected WorktreePath=%s, got %s", repoPath, r.WorktreePath)
	}
}

func TestManager_ImplicitProject_MutationsBlocked(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, "workspace")
	projBase := filepath.Join(dir, "projects")
	_ = os.MkdirAll(wsDir, 0755)
	_ = os.MkdirAll(projBase, 0755)
	m := NewManager(projBase, wsDir)

	setupWorkspaceRepo(t, wsDir, "my-repo")

	if err := m.AddRepo("my-repo", "anything"); err != ErrImmutable {
		t.Errorf("AddRepo: expected ErrImmutable, got %v", err)
	}
	if err := m.RemoveRepo("my-repo", "anything"); err != ErrImmutable {
		t.Errorf("RemoveRepo: expected ErrImmutable, got %v", err)
	}
	if err := m.DeleteProject("my-repo"); err != ErrImmutable {
		t.Errorf("DeleteProject: expected ErrImmutable, got %v", err)
	}
	if _, err := m.EnsurePRWorktree("my-repo", "my-repo", 42, "branch"); err != ErrImmutable {
		t.Errorf("EnsurePRWorktree: expected ErrImmutable, got %v", err)
	}
	if err := m.RemovePRWorktree("my-repo", "my-repo", 42); err != ErrImmutable {
		t.Errorf("RemovePRWorktree: expected ErrImmutable, got %v", err)
	}
}

func TestMergePRs_Deduplicates(t *testing.T) {
	a := []PRInfo{
		{Number: 1, Title: "PR one", State: "OPEN"},
		{Number: 2, Title: "PR two", State: "OPEN"},
	}
	b := []PRInfo{
		{Number: 2, Title: "PR two (dup)", State: "OPEN"},
		{Number: 3, Title: "PR three", State: "OPEN"},
	}

	result := mergePRs(a, b)
	if len(result) != 3 {
		t.Fatalf("expected 3 PRs after merge, got %d", len(result))
	}

	// First slice takes precedence for duplicates.
	numbers := make([]int, len(result))
	for i, pr := range result {
		numbers[i] = pr.Number
	}
	if numbers[0] != 1 || numbers[1] != 2 || numbers[2] != 3 {
		t.Errorf("expected PR numbers [1, 2, 3], got %v", numbers)
	}
	// PR 2 should use the title from slice a (first wins).
	if result[1].Title != "PR two" {
		t.Errorf("expected first-wins title 'PR two', got %q", result[1].Title)
	}
}

func TestMergePRs_EmptySlices(t *testing.T) {
	// Both empty.
	result := mergePRs(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(result))
	}

	// One empty.
	a := []PRInfo{{Number: 1, Title: "PR one", State: "OPEN"}}
	result = mergePRs(a, nil)
	if len(result) != 1 || result[0].Number != 1 {
		t.Errorf("expected [1], got %v", result)
	}

	result = mergePRs(nil, a)
	if len(result) != 1 || result[0].Number != 1 {
		t.Errorf("expected [1], got %v", result)
	}
}
