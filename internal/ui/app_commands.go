package ui

import (
	"path/filepath"
	"sync"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/project"

	tea "github.com/charmbracelet/bubbletea"
)

// loadProjectsCmd returns a command that loads projects from disk (phase 1: instant data).
// It loads project names and repo counts from filesystem only (<10ms), then triggers
// async enrichment for PR and bead counts. Returns both ProjectsLoadedMsg (instant)
// and enrichesProjectsCmd (async) for progressive loading.
func loadProjectsCmd(m *project.Manager) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		infos, err := m.ListProjects()
		if err != nil {
			return ProjectsLoadedMsg{Projects: nil}
		}
		// Phase 1: Instant data (filesystem-only)
		projects := make([]ProjectSummary, len(infos))
		for i, info := range infos {
			projects[i] = ProjectSummary{
				Name:      info.Name,
				RepoCount: info.RepoCount,
				PRCount:   -1, // -1 indicates loading/unknown
				BeadCount: -1, // -1 indicates loading/unknown
				Selected:  false,
			}
		}
		return ProjectsLoadedMsg{Projects: projects}
	}
}

// enrichesProjectsCmd returns a command that enriches projects with PR and bead counts (phase 2: async data).
// This runs after the dashboard has rendered with instant data, fetching PRs and beads
// in parallel across repos and resources for optimal performance.
func enrichesProjectsCmd(m *project.Manager, projectInfos []project.ProjectInfo) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectsEnrichedMsg{Projects: nil}
		}
		projects := make([]ProjectSummary, len(projectInfos))

		// Parallelize across projects (each project's data is independent).
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i, info := range projectInfos {
			wg.Add(1)
			go func(idx int, projectName string, repoCount int) {
				defer wg.Done()
				summary := m.LoadProjectSummary(projectName)
				beadCount := countBeadsFromResources(summary.Resources, projectName)

				mu.Lock()
				projects[idx] = ProjectSummary{
					Name:      projectName,
					RepoCount: repoCount,
					PRCount:   summary.PRCount,
					BeadCount: beadCount,
					Selected:  false,
				}
				mu.Unlock()
			}(i, info.Name, info.RepoCount)
		}

		wg.Wait()

		return ProjectsEnrichedMsg{Projects: projects}
	}
}

// loadProjectDetailResourcesCmd returns a command that loads repos instantly (phase 1: instant data).
// This is filesystem-only and returns immediately with repo resources only.
func loadProjectDetailResourcesCmd(m *project.Manager, projectName string) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectDetailResourcesLoadedMsg{ProjectName: projectName, Resources: nil}
		}
		repos, _ := m.ListProjectRepos(projectName)
		projDir := m.ProjectDir(projectName)

		// Phase 1: Instant data (filesystem-only, no network calls)
		resources := make([]project.Resource, 0, len(repos))
		for _, repoName := range repos {
			worktreePath := filepath.Join(projDir, repoName)
			resources = append(resources, project.Resource{
				Kind:         project.ResourceRepo,
				RepoName:     repoName,
				WorktreePath: worktreePath,
			})
		}
		return ProjectDetailResourcesLoadedMsg{ProjectName: projectName, Resources: resources}
	}
}

// loadProjectPRsCmd returns a command that loads PRs asynchronously (phase 2: async data).
// Runs ListProjectPRs in a goroutine and returns ProjectPRsLoadedMsg with PRs grouped by repo.
func loadProjectPRsCmd(m *project.Manager, projectName string) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return ProjectPRsLoadedMsg{ProjectName: projectName, PRsByRepo: nil}
		}
		prsByRepo, _ := m.ListProjectPRs(projectName)
		return ProjectPRsLoadedMsg{ProjectName: projectName, PRsByRepo: prsByRepo}
	}
}

// loadResourceBeadsCmd returns a command that loads beads asynchronously (phase 3: async data).
// Spawns goroutines for each resource with a worktree, calls beads.ListForRepo/beads.ListForPR
// in parallel, and returns ResourceBeadsLoadedMsg with beads grouped by resource index.
func loadResourceBeadsCmd(projectName string, resources []project.Resource) tea.Cmd {
	return func() tea.Msg {
		beadsByResource := make(map[int][]project.BeadInfo)

		// Fetch beads concurrently across resources.
		var wg sync.WaitGroup
		var mu sync.Mutex

		for i := range resources {
			r := &resources[i]
			if r.WorktreePath == "" {
				continue
			}
			wg.Add(1)
			go func(resIdx int) {
				defer wg.Done()
				var bdBeads []beads.Bead
				var err error
				switch resources[resIdx].Kind {
				case project.ResourceRepo:
					bdBeads, err = beads.ListForRepo(resources[resIdx].WorktreePath, projectName)
				case project.ResourcePR:
					if resources[resIdx].PR != nil {
						bdBeads, err = beads.ListForPR(resources[resIdx].WorktreePath, projectName, resources[resIdx].PR.Number)
					}
				}
				if err != nil {
					// Silently ignore errors - TUI should continue functioning
					// Errors are now returned instead of logged, preventing TUI interference
					return
				}
				beadInfos := make([]project.BeadInfo, len(bdBeads))
				for j, b := range bdBeads {
					beadInfos[j] = project.BeadInfo{
						ID:          b.ID,
						Title:       b.Title,
						Description: b.Description,
						Status:      b.Status,
						IssueType:   b.IssueType,
						Labels:      b.Labels,
						IsChild:     b.ParentID != "",
					}
				}
				mu.Lock()
				beadsByResource[resIdx] = beadInfos
				mu.Unlock()
			}(i)
		}

		wg.Wait()

		return ResourceBeadsLoadedMsg{ProjectName: projectName, BeadsByResource: beadsByResource}
	}
}

// tickCmd returns a command that schedules a tickMsg after 5 seconds.
// Used for periodic refresh of panes and beads in project detail view.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// countBeadsFromResources counts open beads across the given resources.
// Used by loadProjectsCmd with resources from LoadProjectSummary to avoid
// a separate ListProjectResources call (which would redundantly fetch PRs).
// Bead counting is parallelized across resources for better performance.
func countBeadsFromResources(resources []project.Resource, projectName string) int {
	if len(resources) == 0 {
		return 0
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	totalCount := 0

	for _, r := range resources {
		if r.WorktreePath == "" {
			continue
		}
		wg.Add(1)
		go func(resource project.Resource) {
			defer wg.Done()
			var count int
			var err error
			switch resource.Kind {
			case project.ResourceRepo:
				var bdBeads []beads.Bead
				bdBeads, err = beads.ListForRepo(resource.WorktreePath, projectName)
				if err == nil {
					count = len(bdBeads)
				}
			case project.ResourcePR:
				if resource.PR != nil {
					var bdBeads []beads.Bead
					bdBeads, err = beads.ListForPR(resource.WorktreePath, projectName, resource.PR.Number)
					if err == nil {
						count = len(bdBeads)
					}
				}
			}
			// Silently ignore errors - TUI should continue functioning
			mu.Lock()
			totalCount += count
			mu.Unlock()
		}(r)
	}

	wg.Wait()
	return totalCount
}
