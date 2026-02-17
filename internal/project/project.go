// Package project provides project management functionality for devdeploy.
// It handles project CRUD operations, worktree management, and PR resource tracking.
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
	"time"

	"devdeploy/internal/worktree"
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

// seededRand is a seeded random number generator for branch naming.
var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// randAlnum returns n random alphanumeric (lowercase) characters.
func randAlnum(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alnumChars[seededRand.Intn(len(alnumChars))]
	}
	return string(b)
}

// resolveDefaultBranch finds the default branch for a repository.
// It first tries to use git symbolic-ref to get the remote HEAD,
// then falls back to common branch names (main, master).
func resolveDefaultBranch(repoPath string) (string, error) {
	// Try to get the default branch from origin/HEAD
	cmd := exec.Command("git", "-C", repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err == nil {
		// Output is like "refs/remotes/origin/main" - extract the branch name
		ref := strings.TrimSpace(string(out))
		// Use the full remote ref (origin/main) for reliability
		if strings.HasPrefix(ref, "refs/remotes/") {
			return strings.TrimPrefix(ref, "refs/remotes/"), nil
		}
	}

	// Fallback: try common default branch names
	candidates := []string{"origin/main", "main", "origin/master", "master"}
	for _, candidate := range candidates {
		if exec.Command("git", "-C", repoPath, "rev-parse", "--verify", candidate).Run() == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("cannot find default branch (tried origin/HEAD, main, master)")
}

// prCacheEntry holds cached PR data with a timestamp.
type prCacheEntry struct {
	prs       []PRInfo
	timestamp time.Time
}

// Manager handles project CRUD and worktree operations.
type Manager struct {
	projectsBase string
	workspace    string
	prCache      map[string]prCacheEntry // key: worktreePath + state + limit
	prCacheMu    sync.RWMutex            // protects prCache
}

// prCacheTTL is how long cached PR data remains valid.
const prCacheTTL = 45 * time.Second

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
		// prCache is nil by default; initialized lazily on first use
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

// ListProjectReposOnly returns repo-only resources (filesystem scan, no GitHub API calls).
// This is a fast, synchronous operation that returns immediately with repo resources.
func (m *Manager) ListProjectReposOnly(projectName string) []Resource {
	repos, _ := m.ListProjectRepos(projectName)
	projDir := m.projectDir(projectName)

	resources := make([]Resource, 0, len(repos))
	for _, repoName := range repos {
		worktreePath := filepath.Join(projDir, repoName)
		resources = append(resources, Resource{
			Kind:         ResourceRepo,
			RepoName:     repoName,
			WorktreePath: worktreePath,
		})
	}
	return resources
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
	defer func() { _ = os.RemoveAll(emptyHooksDir) }()
	gitNoHooks := []string{"-C", srcRepo, "-c", "core.hooksPath=" + emptyHooksDir}

	// Fetch to ensure we have latest default branch
	fetchCmd := exec.Command("git", "-C", srcRepo, "fetch", "origin")
	fetchCmd.Stderr = nil
	// Best-effort fetch; failure is okay if we already have the ref locally
	_ = fetchCmd.Run()

	// Resolve default branch ref
	mainRef, err := resolveDefaultBranch(srcRepo)
	if err != nil {
		return err
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
		if err := InjectWorktreeRules(dstPath); err != nil {
			return err
		}
		// Invalidate cache for this project since a repo was added
		m.ClearPRCacheForProject(projectName)
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
	if err := InjectWorktreeRules(dstPath); err != nil {
		return err
	}
	// Invalidate cache for this project since a repo was added
	m.ClearPRCacheForProject(projectName)
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
	// Invalidate cache for this project since a repo was removed
	m.ClearPRCacheForProject(projectName)
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

	// Invalidate cache for this project since a worktree was removed
	m.ClearPRCacheForProject(projectName)
	return nil
}

func (m *Manager) projectDir(name string) string {
	normalized := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return filepath.Join(m.projectsBase, normalized)
}

// ProjectDir returns the project directory path for a given project name.
// This is exported for use in async loading scenarios.
func (m *Manager) ProjectDir(name string) string {
	return m.projectDir(name)
}

// PRInfo holds minimal PR metadata from gh pr list.
type PRInfo struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	HeadRefName string     `json:"headRefName"` // branch name for worktree checkout
	MergedAt    *time.Time `json:"mergedAt"`    // when the PR was merged (nil if not merged)
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

// prCacheKey generates a cache key from worktreePath, state, and limit.
func prCacheKey(worktreePath, state string, limit int) string {
	return fmt.Sprintf("%s:%s:%d", worktreePath, state, limit)
}

// getCachedPRs returns cached PR data if it exists and is still valid.
func (m *Manager) getCachedPRs(key string) ([]PRInfo, bool) {
	m.prCacheMu.RLock()
	if m.prCache == nil {
		m.prCacheMu.RUnlock()
		return nil, false
	}
	entry, ok := m.prCache[key]
	m.prCacheMu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Since(entry.timestamp) > prCacheTTL {
		return nil, false
	}
	// Return a copy to avoid external mutation
	result := make([]PRInfo, len(entry.prs))
	copy(result, entry.prs)
	return result, true
}

// setCachedPRs stores PR data in the cache.
func (m *Manager) setCachedPRs(key string, prs []PRInfo) {
	m.prCacheMu.Lock()
	defer m.prCacheMu.Unlock()
	if m.prCache == nil {
		m.prCache = make(map[string]prCacheEntry)
	}
	// Store a copy to avoid external mutation
	cached := make([]PRInfo, len(prs))
	copy(cached, prs)
	m.prCache[key] = prCacheEntry{
		prs:       cached,
		timestamp: time.Now(),
	}
}

// ClearPRCache clears all cached PR data. Call this on manual refresh.
func (m *Manager) ClearPRCache() {
	m.prCacheMu.Lock()
	defer m.prCacheMu.Unlock()
	m.prCache = nil
}

// ClearPRCacheForProject clears cached PR data for a specific project.
// This is called when worktrees are created/deleted for a project.
func (m *Manager) ClearPRCacheForProject(projectName string) {
	m.prCacheMu.Lock()
	defer m.prCacheMu.Unlock()
	if m.prCache == nil {
		return
	}
	projDir := m.projectDir(projectName)
	// Remove all cache entries for worktrees in this project
	for key := range m.prCache {
		if strings.HasPrefix(key, projDir) {
			delete(m.prCache, key)
		}
	}
}

// listPRsInRepo runs gh pr list in the given worktree dir and returns PRs.
// state: "open", "merged", "closed", or "all". limit: max PRs (0 = default 30).
// extraArgs are appended to the gh command (e.g. --author, --search).
func (m *Manager) listPRsInRepo(worktreePath string, state string, limit int, extraArgs ...string) ([]PRInfo, error) {
	args := []string{"pr", "list", "--json", "number,title,state,headRefName,mergedAt"}
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
// Results are cached for prCacheTTL to reduce GitHub API calls.
func (m *Manager) listFilteredPRsInRepo(worktreePath string, state string, limit int) ([]PRInfo, error) {
	// Check cache first
	cacheKey := prCacheKey(worktreePath, state, limit)
	if cached, ok := m.getCachedPRs(cacheKey); ok {
		return cached, nil
	}

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

	result := mergePRs(myPRs, teamPRs)
	// Cache successful results
	m.setCachedPRs(cacheKey, result)
	return result, nil
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
// This is a wrapper around LoadPRs for backward compatibility.
func (m *Manager) CountPRs(projectName string) int {
	result, err := m.LoadPRs(projectName, PRLoadOptions{
		State:    "open",
		Filtered: true,
	})
	if err != nil {
		return 0
	}
	return result.PRCount
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
// This is a wrapper around LoadPRs for backward compatibility.
func (m *Manager) LoadProjectSummary(projectName string) DashboardSummary {
	result, err := m.LoadPRs(projectName, PRLoadOptions{
		State:         "open",
		Filtered:      true,
		BuildResources: true,
	})
	if err != nil {
		return DashboardSummary{PRCount: 0, Resources: []Resource{}}
	}
	return DashboardSummary{PRCount: result.PRCount, Resources: result.Resources}
}

// mergedPRsLimit is how many recently merged PRs to show per repo.
const mergedPRsLimit = 5

// mergedPRMaxAge is the maximum age of merged PRs to show (20 hours).
const mergedPRMaxAge = 20 * time.Hour

// PR Loading Consolidation
//
// All PR loading goes through LoadPRs(), which is the unified method for fetching PRs.
// The following convenience wrapper methods exist for backward compatibility and
// specific use cases:
//
//   - CountPRs() - returns PR count only
//   - LoadProjectSummary() - returns DashboardSummary (PRCount + Resources) for dashboard
//   - ListProjectPRs() - returns []RepoPRs grouped by repository
//   - ListProjectResources() - returns []Resource (repos + PRs) for detail view
//
// All wrappers use LoadPRs() internally, ensuring consistent behavior and caching.
// PR fetching is parallelized across repos, and within each repo, open and merged
// PRs are fetched concurrently when both are requested.

// PRFormat specifies the output format for LoadPRs.
type PRFormat int

const (
	// FormatGrouped returns PRs grouped by repository ([]RepoPRs).
	FormatGrouped PRFormat = iota
	// FormatFlat returns a flat list of resources ([]Resource).
	FormatFlat
	// FormatCount returns only the count (requires CountOnly=true).
	FormatCount
)

// PRLoadOptions configures how PRs are loaded.
type PRLoadOptions struct {
	// RepoNames optionally filters to specific repos. If empty, loads for all repos.
	RepoNames []string

	// State filters PRs by state: "open", "merged", "closed", or "all".
	// Empty string defaults to "open". When IncludeOpen is true, this is used for the primary state.
	State string

	// Limit is the maximum number of PRs to fetch per repo (0 = default 30).
	Limit int

	// IncludeOpen includes open PRs (default: true).
	IncludeOpen bool
	// IncludeMerged includes merged PRs (default: false).
	IncludeMerged bool
	// MergedLimit limits the number of merged PRs per repo (default: 5, only applies if IncludeMerged is true).
	MergedLimit int
	// MergedMaxAge filters merged PRs by age (default: 20 hours, only applies if IncludeMerged is true).
	MergedMaxAge time.Duration
	// Format specifies the output format (default: FormatGrouped).
	Format PRFormat
	// IncludeRepos includes repo resources in the result (default: false, for resource lists).
	IncludeRepos bool
	// CountOnly returns only the count, no PR details (default: false, requires Format=FormatCount).
	CountOnly bool

	// Filtered controls whether to filter PRs by author (@me) and team review requests.
	// When true, uses listFilteredPRsInRepo; when false, uses listPRsInRepo.
	Filtered bool

	// BuildResources controls whether to build Resource list from repos and PRs.
	// When true, Resources field in PRLoadResult will be populated.
	BuildResources bool
}

// PRLoadResult holds the results of loading PRs.
type PRLoadResult struct {
	// PRCount is the total number of PRs across all repos.
	PRCount int
	// PRsByRepo contains PRs grouped by repository (populated if Format=FormatGrouped).
	PRsByRepo []RepoPRs
	// Resources contains a flat list of resources (populated if Format=FormatFlat or IncludeRepos=true or BuildResources=true).
	Resources []Resource
}

// LoadPRs loads PRs according to the provided options.
// This is the unified API for loading PRs; existing methods are wrappers around this.
func (m *Manager) LoadPRs(projectName string, opts PRLoadOptions) (PRLoadResult, error) {
	if projectName == "" {
		return PRLoadResult{}, fmt.Errorf("projectName is required")
	}

	// Apply defaults
	if opts.Format == 0 {
		opts.Format = FormatGrouped
	}
	if !opts.IncludeOpen && !opts.IncludeMerged {
		opts.IncludeOpen = true // default
	}
	if opts.MergedLimit == 0 && opts.IncludeMerged {
		opts.MergedLimit = mergedPRsLimit
	}
	if opts.MergedMaxAge == 0 && opts.IncludeMerged {
		opts.MergedMaxAge = mergedPRMaxAge
	}

	// Resolve repos to load
	var repos []string
	var err error
	if len(opts.RepoNames) > 0 {
		repos = opts.RepoNames
	} else {
		repos, err = m.ListProjectRepos(projectName)
		if err != nil {
			return PRLoadResult{}, err
		}
	}

	if len(repos) == 0 {
		return PRLoadResult{
			PRsByRepo: []RepoPRs{},
			Resources: []Resource{},
			PRCount:   0,
		}, nil
	}

	// Set defaults for state and limit
	state := opts.State
	if state == "" {
		state = "open"
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 30 // gh pr list default
	}
	mergedLimit := opts.MergedLimit
	if mergedLimit == 0 {
		mergedLimit = mergedPRsLimit
	}
	mergedMaxAge := opts.MergedMaxAge
	if mergedMaxAge == 0 {
		mergedMaxAge = mergedPRMaxAge
	}

	projDir := m.projectDir(projectName)

	// Parallelize across repos
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

			var allPRs []PRInfo
			var primaryErr error

			// Determine what to fetch
			shouldFetchOpen := opts.IncludeOpen && (state == "open" || state == "")
			shouldFetchMerged := opts.IncludeMerged

			if shouldFetchOpen && shouldFetchMerged {
				// Fetch open and merged PRs concurrently
				var openPRs []PRInfo
				var mergedPRs []PRInfo
				var mergedErr error

				var prWg sync.WaitGroup
				prWg.Add(2)

				go func() {
					defer prWg.Done()
					if opts.Filtered {
						openPRs, primaryErr = m.listFilteredPRsInRepo(worktreePath, "open", limit)
					} else {
						openPRs, primaryErr = m.listPRsInRepo(worktreePath, "open", limit)
					}
				}()

				go func() {
					defer prWg.Done()
					if opts.Filtered {
						mergedPRs, mergedErr = m.listFilteredPRsInRepo(worktreePath, "merged", mergedLimit)
					} else {
						mergedPRs, mergedErr = m.listPRsInRepo(worktreePath, "merged", mergedLimit)
					}
				}()

				prWg.Wait()

				// Combine results
				if primaryErr == nil {
					allPRs = append(allPRs, openPRs...)
				}
				if mergedErr == nil {
					// Filter merged PRs by age
					cutoff := time.Now().Add(-mergedMaxAge)
					for _, pr := range mergedPRs {
						if pr.MergedAt != nil && pr.MergedAt.After(cutoff) {
							allPRs = append(allPRs, pr)
						}
					}
				}
			} else if shouldFetchOpen {
				// Fetch only open PRs
				if opts.Filtered {
					allPRs, primaryErr = m.listFilteredPRsInRepo(worktreePath, "open", limit)
				} else {
					allPRs, primaryErr = m.listPRsInRepo(worktreePath, "open", limit)
				}
			} else if shouldFetchMerged {
				// Fetch only merged PRs
				var mergedPRs []PRInfo
				if opts.Filtered {
					mergedPRs, primaryErr = m.listFilteredPRsInRepo(worktreePath, "merged", mergedLimit)
				} else {
					mergedPRs, primaryErr = m.listPRsInRepo(worktreePath, "merged", mergedLimit)
				}
				if primaryErr == nil {
					// Filter merged PRs by age
					cutoff := time.Now().Add(-mergedMaxAge)
					for _, pr := range mergedPRs {
						if pr.MergedAt != nil && pr.MergedAt.After(cutoff) {
							allPRs = append(allPRs, pr)
						}
					}
				}
			} else if state != "" && state != "open" {
				// Fallback: use state if specified (for backward compatibility)
				if opts.Filtered {
					allPRs, primaryErr = m.listFilteredPRsInRepo(worktreePath, state, limit)
				} else {
					allPRs, primaryErr = m.listPRsInRepo(worktreePath, state, limit)
				}
			}

			resultChan <- repoResult{repoName: name, prs: allPRs, err: primaryErr}
		}(repoName)
	}

	// Close channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	prsByRepo := make(map[string][]PRInfo)
	for result := range resultChan {
		if result.err == nil && len(result.prs) > 0 {
			prsByRepo[result.repoName] = result.prs
		}
	}

	// Build RepoPRs for backward compatibility and FormatGrouped
	var repoPRs []RepoPRs
	for _, repoName := range repos {
		if prs, ok := prsByRepo[repoName]; ok {
			repoPRs = append(repoPRs, RepoPRs{Repo: repoName, PRs: prs})
		} else if opts.Format == FormatGrouped {
			// Include repos with no PRs for FormatGrouped
			repoPRs = append(repoPRs, RepoPRs{Repo: repoName, PRs: []PRInfo{}})
		}
	}

	// Calculate PR count
	prCount := 0
	for _, prs := range prsByRepo {
		prCount += len(prs)
	}

	// Build resources if requested
	var resources []Resource
	if opts.BuildResources || opts.IncludeRepos || opts.Format == FormatFlat {
		resources = m.buildResourcesFromReposAndPRs(repos, projDir, prsByRepo)
	}

	result := PRLoadResult{
		PRCount:   prCount,
		Resources: resources,
	}

	// Set PRsByRepo based on format
	if opts.Format == FormatGrouped {
		result.PRsByRepo = repoPRs
	} else {
		result.PRsByRepo = []RepoPRs{}
	}

	return result, nil
}

// ListProjectPRs returns PRs grouped by repo (open + recently merged).
// PRs are fetched in parallel across repos, and within each repo, open and merged PRs
// are fetched concurrently for optimal performance.
// This is a wrapper around LoadPRs for backward compatibility.
func (m *Manager) ListProjectPRs(projectName string) ([]RepoPRs, error) {
	result, err := m.LoadPRs(projectName, PRLoadOptions{
		State:         "open",
		IncludeMerged: true,
		MergedLimit:   mergedPRsLimit,
		MergedMaxAge:  mergedPRMaxAge,
		Filtered:      true,
		Format:        FormatGrouped,
	})
	if err != nil {
		return nil, err
	}
	return result.PRsByRepo, nil
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
			// Ignore injection errors: rules are best-effort convenience for existing worktrees.
			// The worktree is usable even if rule injection fails.
			_ = InjectWorktreeRules(dstPath)
			return dstPath, nil // Reusing existing worktree, no cache invalidation needed
		}
	}

	// Scan existing worktrees for one already on this branch.
	if existing := worktree.FindWorktreeForBranch(srcRepo, branchName, true); existing != "" {
		// Ignore injection errors: rules are best-effort convenience for existing worktrees.
		// The worktree is usable even if rule injection fails.
		_ = InjectWorktreeRules(existing)
		return existing, nil // Reusing existing worktree, no cache invalidation needed
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
	defer func() { _ = os.RemoveAll(emptyHooksDir) }()
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

	// Invalidate cache for this project since a new worktree was created
	m.ClearPRCacheForProject(projectName)
	return dstPath, nil
}

// buildResourcesFromReposAndPRs builds a flat []Resource from repos and PRs.
// Resources are ordered repo-first: each repo Resource is followed by its PR Resources.
// prsByRepo maps repo names to their PRs (may be empty for repos with no PRs).
func (m *Manager) buildResourcesFromReposAndPRs(repos []string, projDir string, prsByRepo map[string][]PRInfo) []Resource {
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

	// Append PR resources in repo order (matching the repo resource order).
	for _, repoName := range repos {
		prs := prsByRepo[repoName]
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

	return resources
}


// ListProjectResources builds a flat []Resource from repos and PRs (open + merged).
// Resources are ordered repo-first: each repo Resource is followed by
// its PR Resources, enabling tree-style rendering in the UI.
// Use this for the project detail view where merged PRs are displayed.
// This is a wrapper around LoadPRs for backward compatibility.
func (m *Manager) ListProjectResources(projectName string) []Resource {
	result, err := m.LoadPRs(projectName, PRLoadOptions{
		State:         "open",
		IncludeMerged: true,
		MergedLimit:   mergedPRsLimit,
		MergedMaxAge:  mergedPRMaxAge,
		Filtered:      true,
		BuildResources: true,
	})
	if err != nil {
		return []Resource{}
	}
	return result.Resources
}
