package ui

import (
	"sync"
	"time"

	"devdeploy/internal/beads"
	"devdeploy/internal/project"

	tea "github.com/charmbracelet/bubbletea"
)

// loadHomeRepoNamesCmd loads repo names quickly from filesystem data only.
func loadHomeRepoNamesCmd(m *project.Manager) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return HomeRepoNamesLoadedMsg{Groups: nil}
		}
		repos, err := m.ListWorkspaceRepos()
		if err != nil {
			return HomeRepoNamesLoadedMsg{Groups: nil, Err: err}
		}
		groups := make([]project.RepoGroup, 0, len(repos))
		for _, repoName := range repos {
			repoPath := m.WorkspaceRepoPath(repoName)
			groups = append(groups, project.RepoGroup{
				RepoName:      repoName,
				WorkspacePath: repoPath,
				Items: []project.Resource{
					{
						Kind:         project.ResourceRepo,
						RepoName:     repoName,
						WorktreePath: repoPath,
					},
				},
			})
		}
		return HomeRepoNamesLoadedMsg{Groups: groups}
	}
}

// loadHomeRepoGroupsCmd loads complete repo groups with worktrees and PRs.
func loadHomeRepoGroupsCmd(m *project.Manager) tea.Cmd {
	return func() tea.Msg {
		if m == nil {
			return HomeRepoGroupsLoadedMsg{Groups: nil}
		}
		groups, err := m.ListRepoGroups()
		if err != nil {
			return HomeRepoGroupsLoadedMsg{Groups: nil, Err: err}
		}
		return HomeRepoGroupsLoadedMsg{Groups: groups}
	}
}

// loadHomeBeadsCmd loads bead data for all group items with worktrees.
func loadHomeBeadsCmd(groups []project.RepoGroup) tea.Cmd {
	return func() tea.Msg {
		out := cloneRepoGroups(groups)

		var wg sync.WaitGroup
		for gi := range out {
			for ii := range out[gi].Items {
				item := &out[gi].Items[ii]
				if item.WorktreePath == "" {
					// Repo headers can be rendered before any worktree exists.
					continue
				}
				wg.Add(1)
				go func(groupIdx, itemIdx int) {
					defer wg.Done()
					resource := out[groupIdx].Items[itemIdx]
					var bdBeads []beads.Bead
					var err error
					switch resource.Kind {
					case project.ResourceRepo, project.ResourceWorktree:
						bdBeads, err = beads.ListForRepo(resource.WorktreePath, out[groupIdx].RepoName)
					case project.ResourcePR:
						if resource.PR != nil {
							bdBeads, err = beads.ListForPR(resource.WorktreePath, out[groupIdx].RepoName, resource.PR.Number)
						}
					}
					if err != nil {
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
					out[groupIdx].Items[itemIdx].Beads = beadInfos
				}(gi, ii)
			}
		}
		wg.Wait()

		return HomeBeadsLoadedMsg{Groups: out}
	}
}

// cloneRepoGroups deep-copies groups so bead loaders can mutate concurrently without races.
func cloneRepoGroups(groups []project.RepoGroup) []project.RepoGroup {
	out := make([]project.RepoGroup, len(groups))
	for i, g := range groups {
		items := make([]project.Resource, len(g.Items))
		for j, item := range g.Items {
			items[j] = item
			if item.PR != nil {
				prCopy := *item.PR
				items[j].PR = &prCopy
			}
			if item.Worktree != nil {
				wtCopy := *item.Worktree
				items[j].Worktree = &wtCopy
			}
			if len(item.Panes) > 0 {
				panesCopy := make([]project.PaneInfo, len(item.Panes))
				copy(panesCopy, item.Panes)
				items[j].Panes = panesCopy
			}
			if len(item.Beads) > 0 {
				beadsCopy := make([]project.BeadInfo, len(item.Beads))
				copy(beadsCopy, item.Beads)
				for bi := range beadsCopy {
					if len(item.Beads[bi].Labels) > 0 {
						labels := make([]string, len(item.Beads[bi].Labels))
						copy(labels, item.Beads[bi].Labels)
						beadsCopy[bi].Labels = labels
					}
				}
				items[j].Beads = beadsCopy
			}
		}
		out[i] = project.RepoGroup{
			RepoName:      g.RepoName,
			WorkspacePath: g.WorkspacePath,
			Items:         items,
		}
	}
	return out
}

// tickCmd returns a command that schedules a periodic refresh tick.
func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
