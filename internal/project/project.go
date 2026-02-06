package project

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// WorkspaceEnv is the env var override for ~/workspace (repos source).
	WorkspaceEnv = "DEVDEPLOY_WORKSPACE"
	// DefaultWorkspace is the default path for listing available repos.
	DefaultWorkspace = "workspace"
)

const alnumChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// randAlnum returns n random alphanumeric (lowercase) characters.
func randAlnum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alnumChars[rand.Intn(len(alnumChars))]
	}
	return string(b)
}

// Manager handles project CRUD and worktree operations.
type Manager struct {
	projectsBase string
	workspace    string
}

// NewManager creates a manager using the same base as artifact.Store.
func NewManager(projectsBase, workspace string) *Manager {
	if workspace == "" {
		workspace = os.Getenv(WorkspaceEnv)
	}
	if workspace == "" {
		home, _ := os.UserHomeDir()
		workspace = filepath.Join(home, DefaultWorkspace)
	}
	return &Manager{
		projectsBase: projectsBase,
		workspace:    workspace,
	}
}

// ProjectInfo holds minimal project metadata for listing.
type ProjectInfo struct {
	Name      string
	RepoCount int
	Dir       string
}

// ListProjects returns projects from disk (~/.devdeploy/projects/).
func (m *Manager) ListProjects() ([]ProjectInfo, error) {
	entries, err := os.ReadDir(m.projectsBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ProjectInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		dir := filepath.Join(m.projectsBase, name)
		repos, _ := m.ListProjectRepos(name)
		out = append(out, ProjectInfo{
			Name:      name,
			RepoCount: len(repos),
			Dir:       dir,
		})
	}
	return out, nil
}

// CreateProject creates a project directory and minimal config.
func (m *Manager) CreateProject(name string) error {
	dir := m.projectDir(name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // already exists
	}
	return os.WriteFile(configPath, []byte("# devdeploy project config\n"), 0644)
}

// DeleteProject removes a project directory and all its worktrees.
// It first runs 'git worktree remove' for each worktree so the main repo
// in ~/workspace does not retain orphaned worktree entries.
func (m *Manager) DeleteProject(name string) error {
	dir := m.projectDir(name)
	repos, err := m.ListProjectRepos(name)
	if err != nil {
		return err
	}
	for _, repoName := range repos {
		if err := m.RemoveRepo(name, repoName); err != nil {
			return fmt.Errorf("remove worktree %s: %w", repoName, err)
		}
	}
	return os.RemoveAll(dir)
}

// ListWorkspaceRepos returns git repo names in ~/workspace.
func (m *Manager) ListWorkspaceRepos() ([]string, error) {
	entries, err := os.ReadDir(m.workspace)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		repoPath := filepath.Join(m.workspace, e.Name(), ".git")
		if info, err := os.Stat(repoPath); err == nil && info.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// ListProjectRepos returns worktree subdir names in the project.
func (m *Manager) ListProjectRepos(projectName string) ([]string, error) {
	dir := m.projectDir(projectName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "config.yaml" {
			continue
		}
		// Skip known artifact files (they're files, not dirs, but be safe)
		// plan.md, design.md, config.yaml are files; skip non-dirs
		subPath := filepath.Join(dir, name)
		if info, err := os.Stat(subPath); err == nil && info.IsDir() {
			// Check it's a git worktree (has .git file pointing to main repo)
			gitPath := filepath.Join(subPath, ".git")
			if _, err := os.Stat(gitPath); err == nil {
				out = append(out, name)
			}
		}
	}
	return out, nil
}

// AddRepo creates a worktree in the project dir from a repo in ~/workspace.
// It creates a new branch named devdeploy/<project>-<3 random alphanumeric chars> based on main,
// ensuring it's up to date. The random suffix reduces collisions when multiple devdeploy
// instances or users add the same project.
// Does not change the main repo's current branch.
// Hooks are disabled during worktree add/merge to avoid repo-specific hooks (e.g. beads)
// from failing and blocking the operation.
func (m *Manager) AddRepo(projectName, repoName string) error {
	srcRepo := filepath.Join(m.workspace, repoName)
	dstPath := filepath.Join(m.projectDir(projectName), repoName)
	if _, err := os.Stat(srcRepo); err != nil {
		return fmt.Errorf("source repo %s: %w", srcRepo, err)
	}
	base := strings.ToLower(strings.ReplaceAll(projectName, " ", "-"))
	branch := "devdeploy/" + base + "-" + randAlnum(3)

	// Empty dir for core.hooksPath to disable hooks (avoids post-checkout etc. failing)
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(emptyHooksDir)
	gitNoHooks := []string{"-C", srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Fetch to ensure we have latest main
	fetchCmd := exec.Command("git", "-C", srcRepo, "fetch", "origin")
	fetchCmd.Stderr = nil
	_ = fetchCmd.Run()

	// Resolve main ref (origin/main or main)
	mainRef := "origin/main"
	if err := exec.Command("git", "-C", srcRepo, "rev-parse", "--verify", mainRef).Run(); err != nil {
		mainRef = "main"
		if err := exec.Command("git", "-C", srcRepo, "rev-parse", "--verify", mainRef).Run(); err != nil {
			return fmt.Errorf("cannot find main branch (tried origin/main, main)")
		}
	}

	var addStderr bytes.Buffer
	if err := exec.Command("git", "-C", srcRepo, "rev-parse", "--verify", branch).Run(); err != nil {
		// Branch doesn't exist: create it when adding worktree (-b creates branch from mainRef)
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", branch, dstPath, mainRef)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("git worktree add: %s", msg)
		}
		return nil
	}

	// Branch exists: add worktree, then update it with main (without touching main repo's HEAD)
	addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", dstPath, branch)...)
	addCmd.Stderr = &addStderr
	if err := addCmd.Run(); err != nil {
		msg := strings.TrimSpace(addStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree add: %s", msg)
	}
	// Update the new worktree's branch with main (disable hooks for merge too)
	mergeNoHooks := []string{"-C", dstPath, "-c", "core.hooksPath=" + emptyHooksDir}
	mergeCmd := exec.Command("git", append(mergeNoHooks, "merge", mainRef, "--no-edit")...)
	mergeCmd.Stderr = &addStderr
	if err := mergeCmd.Run(); err != nil {
		msg := strings.TrimSpace(addStderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git merge %s (in worktree): %s", mainRef, msg)
	}
	return nil
}

// RemoveRepo removes a worktree from the project.
func (m *Manager) RemoveRepo(projectName, repoName string) error {
	srcRepo := filepath.Join(m.workspace, repoName)
	worktreePath := filepath.Join(m.projectDir(projectName), repoName)
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "remove", worktreePath, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree remove: %s", msg)
	}
	return nil
}

func (m *Manager) projectDir(name string) string {
	normalized := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return filepath.Join(m.projectsBase, normalized)
}
