package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	// WorkspaceEnv is the env var override for ~/workspace (repos source).
	WorkspaceEnv = "DEVDEPLOY_WORKSPACE"
	// DefaultWorkspace is the default path for listing available repos.
	DefaultWorkspace = "workspace"

	// ProjectDirEnv is the env var override for the projects base directory.
	ProjectDirEnv = "DEVDEPLOY_PROJECTS_DIR"
	// DefaultProjectsBase is the default base for project directories under $HOME.
	DefaultProjectsBase = ".devdeploy/projects"
)

// ResolveProjectsBase returns the projects base directory, using the
// DEVDEPLOY_PROJECTS_DIR env var if set, otherwise ~/.devdeploy/projects.
func ResolveProjectsBase() (string, error) {
	if base := os.Getenv(ProjectDirEnv); base != "" {
		return base, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DefaultProjectsBase), nil
}

const alnumChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// prWorktreePattern matches PR worktree directory names: <repo>-pr-<number>.
var prWorktreePattern = regexp.MustCompile(`^.+-pr-\d+$`)

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

// NewManager creates a manager for the given projects base directory.
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
		// Skip PR worktree dirs (e.g. my-repo-pr-42); they belong to PR resources.
		if prWorktreePattern.MatchString(name) {
			continue
		}
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
		return InjectWorktreeRules(dstPath)
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
	return InjectWorktreeRules(dstPath)
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

// RemovePRWorktree removes a PR worktree from the project.
// The worktree directory is <projectDir>/<repoName>-pr-<number>.
// If the worktree directory does not exist, this is a no-op.
func (m *Manager) RemovePRWorktree(projectName, repoName string, prNumber int) error {
	wtName := fmt.Sprintf("%s-pr-%d", repoName, prNumber)
	wtPath := filepath.Join(m.projectDir(projectName), wtName)

	// If worktree dir doesn't exist, nothing to do.
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return nil
	}

	srcRepo := filepath.Join(m.workspace, repoName)
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "remove", wtPath, "--force")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("git worktree remove (PR): %s", msg)
	}
	return nil
}

func (m *Manager) projectDir(name string) string {
	normalized := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return filepath.Join(m.projectsBase, normalized)
}

// PRInfo holds minimal PR metadata from gh pr list.
type PRInfo struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	HeadRefName string `json:"headRefName"` // branch name for worktree checkout
}

// RepoPRs groups PRs by repository for display.
type RepoPRs struct {
	Repo string
	PRs  []PRInfo
}

// reviewTeam is the GitHub team slug used to filter PRs by review request.
// PRs requesting review from this team are included alongside the current
// user's own PRs.
const reviewTeam = "adaptive-telemetry"

// listPRsInRepo runs gh pr list in the given worktree dir and returns PRs.
// state: "open", "merged", "closed", or "all". limit: max PRs (0 = default 30).
// extraArgs are appended to the gh command (e.g. --author, --search).
func (m *Manager) listPRsInRepo(worktreePath string, state string, limit int, extraArgs ...string) ([]PRInfo, error) {
	args := []string{"pr", "list", "--json", "number,title,state,headRefName"}
	if state != "" && state != "open" {
		args = append(args, "--state", state)
	}
	if limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", limit))
	}
	args = append(args, extraArgs...)
	cmd := exec.Command("gh", args...)
	cmd.Dir = worktreePath
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	var prs []PRInfo
	if err := json.Unmarshal(out.Bytes(), &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// getRepoOwner returns the GitHub owner (org or user) for a repo worktree
// by running `gh repo view --json owner`. Returns "" on failure.
func getRepoOwner(worktreePath string) string {
	cmd := exec.Command("gh", "repo", "view", "--json", "owner", "-q", ".owner.login")
	cmd.Dir = worktreePath
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// listFilteredPRsInRepo returns PRs authored by the current user OR
// requesting review from the reviewTeam. It makes two gh pr list calls
// and deduplicates the results by PR number.
func (m *Manager) listFilteredPRsInRepo(worktreePath string, state string, limit int) ([]PRInfo, error) {
	// Fetch PRs authored by the current user.
	myPRs, err := m.listPRsInRepo(worktreePath, state, limit, "--author", "@me")

	// Fetch PRs requesting review from the team.
	var teamPRs []PRInfo
	var teamErr error
	if owner := getRepoOwner(worktreePath); owner != "" {
		search := fmt.Sprintf("team-review-requested:%s/%s", owner, reviewTeam)
		teamPRs, teamErr = m.listPRsInRepo(worktreePath, state, limit, "--search", search)
	}

	// If both calls failed, return the first error.
	if err != nil && teamErr != nil {
		return nil, err
	}

	return mergePRs(myPRs, teamPRs), nil
}

// mergePRs combines two PR slices and deduplicates by PR number.
// The first slice's entries take precedence on duplicates.
func mergePRs(a, b []PRInfo) []PRInfo {
	seen := make(map[int]bool, len(a))
	result := make([]PRInfo, 0, len(a)+len(b))
	for _, pr := range a {
		seen[pr.Number] = true
		result = append(result, pr)
	}
	for _, pr := range b {
		if !seen[pr.Number] {
			seen[pr.Number] = true
			result = append(result, pr)
		}
	}
	return result
}

// CountPRs returns the number of open PRs across the project's repos.
func (m *Manager) CountPRs(projectName string) int {
	repos, err := m.ListProjectRepos(projectName)
	if err != nil || len(repos) == 0 {
		return 0
	}
	count := 0
	for _, repoName := range repos {
		worktreePath := filepath.Join(m.projectDir(projectName), repoName)
		prs, err := m.listFilteredPRsInRepo(worktreePath, "open", 0)
		if err != nil {
			continue
		}
		count += len(prs)
	}
	return count
}

// DashboardSummary holds pre-computed data for the dashboard view.
// It is produced by LoadProjectSummary which fetches open PRs once
// per repo, avoiding redundant gh pr list calls.
type DashboardSummary struct {
	PRCount   int        // total open PRs across all repos
	Resources []Resource // repos + open PR resources (no merged PRs)
}

// LoadProjectSummary fetches open PRs once per repo and returns both
// the PR count and a resource list suitable for bead counting.
// Unlike ListProjectResources (which also fetches merged PRs for the
// detail view), this method only fetches open PRs — sufficient for
// the dashboard where merged PRs are not displayed.
// PR fetching is parallelized across repos for better performance.
func (m *Manager) LoadProjectSummary(projectName string) DashboardSummary {
	repos, _ := m.ListProjectRepos(projectName)
	projDir := m.projectDir(projectName)

	// Pre-allocate repo resources (filesystem-only, no network calls).
	resources := make([]Resource, 0, len(repos))
	for _, repoName := range repos {
		worktreePath := filepath.Join(projDir, repoName)
		resources = append(resources, Resource{
			Kind:         ResourceRepo,
			RepoName:     repoName,
			WorktreePath: worktreePath,
		})
	}

	// Fetch PRs concurrently across repos.
	type repoResult struct {
		repoName string
		prs      []PRInfo
		err      error
	}
	resultChan := make(chan repoResult, len(repos))
	var wg sync.WaitGroup

	for _, repoName := range repos {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			worktreePath := filepath.Join(projDir, name)
			prs, err := m.listFilteredPRsInRepo(worktreePath, "open", 0)
			resultChan <- repoResult{repoName: name, prs: prs, err: err}
		}(repoName)
	}

	// Close channel when all goroutines complete.
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results and build resource list.
	prCount := 0
	repoPRs := make(map[string][]PRInfo)
	for result := range resultChan {
		if result.err != nil {
			continue
		}
		repoPRs[result.repoName] = result.prs
		prCount += len(result.prs)
	}

	// Append PR resources in repo order (matching the repo resource order).
	for _, repoName := range repos {
		prs := repoPRs[repoName]
		for i := range prs {
			pr := &prs[i]
			// Check if a PR worktree already exists on disk.
			prWT := filepath.Join(projDir, fmt.Sprintf("%s-pr-%d", repoName, pr.Number))
			var wtPath string
			if info, err := os.Stat(prWT); err == nil && info.IsDir() {
				wtPath = prWT
			}
			resources = append(resources, Resource{
				Kind:         ResourcePR,
				RepoName:     repoName,
				PR:           pr,
				WorktreePath: wtPath,
			})
		}
	}

	return DashboardSummary{PRCount: prCount, Resources: resources}
}

// mergedPRsLimit is how many recently merged PRs to show per repo.
const mergedPRsLimit = 5

// ListProjectPRs returns PRs grouped by repo (open + recently merged).
func (m *Manager) ListProjectPRs(projectName string) ([]RepoPRs, error) {
	repos, err := m.ListProjectRepos(projectName)
	if err != nil {
		return nil, err
	}
	var out []RepoPRs
	for _, repoName := range repos {
		worktreePath := filepath.Join(m.projectDir(projectName), repoName)
		var all []PRInfo
		open, _ := m.listFilteredPRsInRepo(worktreePath, "open", 0)
		all = append(all, open...)
		merged, _ := m.listFilteredPRsInRepo(worktreePath, "merged", mergedPRsLimit)
		all = append(all, merged...)
		if len(all) > 0 {
			out = append(out, RepoPRs{Repo: repoName, PRs: all})
		}
	}
	return out, nil
}

// EnsurePRWorktree creates or reuses a worktree for a PR branch.
// It checks if a worktree already exists for the branch (by scanning
// git worktree list output from the source repo), and if so reuses it.
// Otherwise it fetches the branch from origin and creates a new worktree.
// The worktree path is: <projectDir>/<repoName>-pr-<number>.
// Returns the absolute worktree path.
func (m *Manager) EnsurePRWorktree(projectName, repoName string, prNumber int, branchName string) (string, error) {
	srcRepo := filepath.Join(m.workspace, repoName)
	if _, err := os.Stat(srcRepo); err != nil {
		return "", fmt.Errorf("source repo %s: %w", srcRepo, err)
	}

	wtName := fmt.Sprintf("%s-pr-%d", repoName, prNumber)
	dstPath := filepath.Join(m.projectDir(projectName), wtName)

	// Check if our target worktree path already exists and is a git worktree.
	if info, err := os.Stat(dstPath); err == nil && info.IsDir() {
		gitFile := filepath.Join(dstPath, ".git")
		if _, err := os.Stat(gitFile); err == nil {
			// Ensure rules are injected (idempotent) even for pre-existing worktrees.
			_ = InjectWorktreeRules(dstPath)
			return dstPath, nil
		}
	}

	// Scan existing worktrees for one already on this branch.
	if existing := m.findWorktreeForBranch(srcRepo, branchName); existing != "" {
		_ = InjectWorktreeRules(existing)
		return existing, nil
	}

	// Fetch the branch from origin (it may not exist locally yet).
	fetchCmd := exec.Command("git", "-C", srcRepo, "fetch", "origin", branchName)
	fetchCmd.Stderr = nil
	_ = fetchCmd.Run() // best-effort; branch may already be local

	// Empty dir for core.hooksPath to disable hooks during worktree add.
	emptyHooksDir, err := os.MkdirTemp("", "devdeploy-nohooks")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(emptyHooksDir)
	gitNoHooks := []string{"-C", srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Try the local branch first; fall back to origin/<branch>.
	ref := branchName
	if err := exec.Command("git", "-C", srcRepo, "rev-parse", "--verify", ref).Run(); err != nil {
		ref = "origin/" + branchName
		if err := exec.Command("git", "-C", srcRepo, "rev-parse", "--verify", ref).Run(); err != nil {
			return "", fmt.Errorf("branch %s not found locally or on origin", branchName)
		}
	}

	var addStderr bytes.Buffer
	// If we have a local branch, check it out directly. If only remote, create a tracking branch.
	if ref == branchName {
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", dstPath, branchName)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", fmt.Errorf("git worktree add: %s", msg)
		}
	} else {
		// Create local tracking branch from origin/<branch>
		addCmd := exec.Command("git", append(gitNoHooks, "worktree", "add", "-b", branchName, dstPath, ref)...)
		addCmd.Stderr = &addStderr
		if err := addCmd.Run(); err != nil {
			msg := strings.TrimSpace(addStderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return "", fmt.Errorf("git worktree add: %s", msg)
		}
	}

	if err := InjectWorktreeRules(dstPath); err != nil {
		return "", fmt.Errorf("inject rules: %w", err)
	}
	return dstPath, nil
}

// findWorktreeForBranch scans git worktree list output for a worktree
// that has the given branch checked out. Returns the worktree path or "".
func (m *Manager) findWorktreeForBranch(srcRepo, branchName string) string {
	cmd := exec.Command("git", "-C", srcRepo, "worktree", "list", "--porcelain")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}

	// Porcelain format: blocks separated by blank lines.
	// Each block has: worktree <path>\nHEAD <sha>\nbranch refs/heads/<name>\n
	var currentPath string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		}
		if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			if branch == branchName && currentPath != "" && currentPath != srcRepo {
				return currentPath
			}
		}
	}
	return ""
}

// ListProjectResources builds a flat []Resource from repos and PRs.
// Resources are ordered repo-first: each repo Resource is followed by
// its PR Resources, enabling tree-style rendering in the UI.
func (m *Manager) ListProjectResources(projectName string) []Resource {
	repos, _ := m.ListProjectRepos(projectName)
	prsByRepo, _ := m.ListProjectPRs(projectName)

	prMap := make(map[string][]PRInfo, len(prsByRepo))
	for _, rp := range prsByRepo {
		prMap[rp.Repo] = rp.PRs
	}

	projDir := m.projectDir(projectName)
	var resources []Resource
	for _, repoName := range repos {
		worktreePath := filepath.Join(projDir, repoName)
		resources = append(resources, Resource{
			Kind:         ResourceRepo,
			RepoName:     repoName,
			WorktreePath: worktreePath,
		})
		for i := range prMap[repoName] {
			pr := &prMap[repoName][i]
			// Check if a PR worktree already exists on disk.
			prWT := filepath.Join(projDir, fmt.Sprintf("%s-pr-%d", repoName, pr.Number))
			var wtPath string
			if info, err := os.Stat(prWT); err == nil && info.IsDir() {
				wtPath = prWT
			}
			resources = append(resources, Resource{
				Kind:         ResourcePR,
				RepoName:     repoName,
				PR:           pr,
				WorktreePath: wtPath,
			})
		}
	}
	return resources
}
