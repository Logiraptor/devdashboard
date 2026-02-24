package ui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"devdeploy/internal/project"
)

func testHomeRepoGroups() []project.RepoGroup {
	return []project.RepoGroup{
		{
			RepoName:      "repo-a",
			WorkspacePath: "/workspace/repo-a",
			Items: []project.Resource{
				{
					Kind:         project.ResourceRepo,
					RepoName:     "repo-a",
					WorktreePath: "/workspace/repo-a",
					Beads: []project.BeadInfo{
						{ID: "a-1", Title: "bead-a1", Status: "open"},
					},
					Panes: []project.PaneInfo{{ID: "%1", IsAgent: false}},
				},
				{
					Kind:         project.ResourceWorktree,
					RepoName:     "repo-a",
					WorktreePath: "/workspace/repo-a-wt",
					Worktree:     &project.WorktreeInfo{Branch: "feat", Path: "/workspace/repo-a-wt"},
				},
				{
					Kind:         project.ResourcePR,
					RepoName:     "repo-a",
					WorktreePath: "/workspace/repo-a-pr-42",
					PR:           &project.PRInfo{Number: 42, Title: "Fix thing", State: "OPEN", HeadRefName: "feat"},
					Beads: []project.BeadInfo{
						{ID: "a-2", Title: "bead-a2", Status: "in_progress", IsChild: true},
					},
				},
			},
		},
	}
}

func TestCloneRepoGroups_DeepCopy(t *testing.T) {
	original := testHomeRepoGroups()
	cloned := cloneRepoGroups(original)
	require.Len(t, cloned, 1)

	cloned[0].RepoName = "mutated"
	cloned[0].Items[0].RepoName = "mutated-repo"
	cloned[0].Items[0].PR = &project.PRInfo{Number: 99, Title: "new"}
	cloned[0].Items[1].Worktree.Branch = "mutated-branch"
	cloned[0].Items[0].Panes[0].ID = "%99"
	cloned[0].Items[2].Beads[0].ID = "mutated-bead"

	require.Equal(t, "repo-a", original[0].RepoName)
	require.Equal(t, "repo-a", original[0].Items[0].RepoName)
	require.Nil(t, original[0].Items[0].PR)
	require.Equal(t, "feat", original[0].Items[1].Worktree.Branch)
	require.Equal(t, "%1", original[0].Items[0].Panes[0].ID)
	require.Equal(t, "a-2", original[0].Items[2].Beads[0].ID)
}

func TestHomeView_BuildItems(t *testing.T) {
	v := NewHomeView()
	v.SetRepoGroups(testHomeRepoGroups())

	require.Len(t, v.items, 5) // 3 resources + 2 beads

	require.Equal(t, homeItemTypeResource, v.items[0].itemType)
	require.Equal(t, 0, v.items[0].groupIdx)
	require.Equal(t, 0, v.items[0].resourceIdx)
	require.Equal(t, -1, v.items[0].beadIdx)

	require.Equal(t, homeItemTypeBead, v.items[1].itemType)
	require.Equal(t, 0, v.items[1].groupIdx)
	require.Equal(t, 0, v.items[1].resourceIdx)
	require.Equal(t, 0, v.items[1].beadIdx)

	require.Equal(t, homeItemTypeResource, v.items[2].itemType)
	require.Equal(t, 1, v.items[2].resourceIdx)

	require.Equal(t, homeItemTypeResource, v.items[3].itemType)
	require.Equal(t, 2, v.items[3].resourceIdx)

	require.Equal(t, homeItemTypeBead, v.items[4].itemType)
	require.Equal(t, 2, v.items[4].resourceIdx)
	require.Equal(t, 0, v.items[4].beadIdx)
}

func TestHomeView_SelectedResourceAndBead(t *testing.T) {
	v := NewHomeView()

	require.Nil(t, v.SelectedResource())
	require.Nil(t, v.SelectedBead())

	v.SetRepoGroups(testHomeRepoGroups())
	v.list.Select(0)
	require.Equal(t, project.ResourceRepo, v.SelectedResource().Kind)
	require.Nil(t, v.SelectedBead())

	v.list.Select(1)
	require.Equal(t, "a-1", v.SelectedBead().ID)
	require.Equal(t, project.ResourceRepo, v.SelectedResource().Kind)

	v.list.Select(100)
	require.Nil(t, v.SelectedResource())
	require.Nil(t, v.SelectedBead())

	// stale indices should never panic and should return nil
	v.list.Select(0)
	v.items[0].groupIdx = 99
	require.Nil(t, v.SelectedResource())

	v.list.Select(1)
	v.items[1].resourceIdx = 99
	require.Nil(t, v.SelectedBead())
}

func TestHomeView_ViewRendering(t *testing.T) {
	v := NewHomeView()
	empty := v.View()
	require.Contains(t, empty, "(no repos found in workspace)")

	v.SetRepoGroups(testHomeRepoGroups())
	populated := v.View()
	require.Contains(t, populated, "repo-a/")

	v.loadingGroups = true
	loading := v.View()
	require.Contains(t, loading, v.spinner.View())
}

func TestHomeItem_FilterValue(t *testing.T) {
	tests := []struct {
		name string
		item homeItem
		want string
	}{
		{
			name: "bead",
			item: homeItem{
				itemType: homeItemTypeBead,
				bead:     &project.BeadInfo{ID: "devdeploy-1", Title: "Fix"},
			},
			want: "devdeploy-1 Fix",
		},
		{
			name: "repo",
			item: homeItem{
				itemType: homeItemTypeResource,
				resource: &project.Resource{Kind: project.ResourceRepo, RepoName: "devdeploy"},
			},
			want: "devdeploy",
		},
		{
			name: "worktree",
			item: homeItem{
				itemType: homeItemTypeResource,
				resource: &project.Resource{
					Kind:     project.ResourceWorktree,
					RepoName: "devdeploy",
					Worktree: &project.WorktreeInfo{Branch: "feat", Path: "/tmp/wt"},
				},
			},
			want: "devdeploy feat /tmp/wt",
		},
		{
			name: "pr",
			item: homeItem{
				itemType: homeItemTypeResource,
				resource: &project.Resource{
					Kind:     project.ResourcePR,
					RepoName: "devdeploy",
					PR:       &project.PRInfo{Number: 42, Title: "Add test", HeadRefName: "feat"},
				},
			},
			want: "devdeploy #42 Add test feat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.item.FilterValue())
		})
	}
}
