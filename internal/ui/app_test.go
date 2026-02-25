package ui

import (
	"errors"
	"testing"

	"devdeploy/internal/project"

	"github.com/stretchr/testify/require"
)

func TestNewAppModelUsesHomeView(t *testing.T) {
	m := NewAppModel()
	require.NotNil(t, m.Home)
	require.NotNil(t, m.KeyHandler)
}

func TestAdapterLoadsHomeHeaders(t *testing.T) {
	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	groups := []project.RepoGroup{
		{
			RepoName: "repo-a",
			Items: []project.Resource{
				{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
			},
		},
	}
	_, _ = adapter.Update(HomeRepoNamesLoadedMsg{Groups: groups})
	require.NotNil(t, m.Home)
	require.Len(t, m.Home.RepoGroups(), 1)
	require.Equal(t, "repo-a", m.Home.RepoGroups()[0].RepoName)
}

func TestHandleHomeRepoNamesLoadedStartsPhase2(t *testing.T) {
	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, cmd := adapter.handleHomeRepoNamesLoaded(HomeRepoNamesLoadedMsg{
		Groups: []project.RepoGroup{
			{
				RepoName: "repo-a",
				Items: []project.Resource{
					{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
				},
			},
		},
	})
	require.NotNil(t, cmd)
	require.True(t, m.Home.loadingGroups)
	require.False(t, m.Home.loadingBeads)
	require.Len(t, m.Home.RepoGroups(), 1)
}

func TestHandleHomeRepoGroupsLoadedTransitionsToBeadLoading(t *testing.T) {
	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, cmd := adapter.handleHomeRepoGroupsLoaded(HomeRepoGroupsLoadedMsg{
		Groups: []project.RepoGroup{
			{
				RepoName: "repo-a",
				Items: []project.Resource{
					{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
				},
			},
		},
	})
	require.NotNil(t, cmd)
	require.False(t, m.Home.loadingGroups)
	require.True(t, m.Home.loadingBeads)
}

func TestHandleHomeBeadsLoadedClearsLoadingFlags(t *testing.T) {
	m := NewAppModel()
	m.Home.loadingGroups = true
	m.Home.loadingBeads = true
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, _ = adapter.handleHomeBeadsLoaded(HomeBeadsLoadedMsg{
		Groups: []project.RepoGroup{
			{
				RepoName: "repo-a",
				Items: []project.Resource{
					{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
				},
			},
		},
	})

	require.False(t, m.Home.loadingGroups)
	require.False(t, m.Home.loadingBeads)
}

func TestEnterDispatchesUpsertSessionMsg(t *testing.T) {
	m := NewAppModel()
	m.Home.SetRepoGroups([]project.RepoGroup{
		{
			RepoName: "repo-a",
			Items: []project.Resource{
				{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
			},
		},
	})
	adapter := m.AsTeaModel().(*appModelAdapter)
	_, cmd := adapter.Update(keyMsg("enter"))
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(UpsertResourceSessionMsg)
	require.True(t, ok)
}

func TestShowRemoveResourceRequiresSelection(t *testing.T) {
	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)
	_, _ = adapter.Update(ShowRemoveResourceMsg{})
	require.Equal(t, "No resource selected", m.Status)
	require.True(t, m.StatusIsError)
}

func TestDKeyIgnoredWhileFiltering(t *testing.T) {
	m := NewAppModel()
	m.Home.SetRepoGroups([]project.RepoGroup{
		{
			RepoName: "repo-a",
			Items: []project.Resource{
				{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
			},
		},
	})
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, _ = adapter.Update(keyMsg("/"))
	require.True(t, m.Home.IsFiltering())

	_, _ = adapter.Update(keyMsg("d"))
	require.Equal(t, 0, m.Overlays.Len())
}

func TestHandleHomeTickRefreshedSetsPreview(t *testing.T) {
	m := NewAppModel()
	m.Home.SetRepoGroups([]project.RepoGroup{
		{
			RepoName: "repo-a",
			Items: []project.Resource{
				{Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/repo-a"},
			},
		},
	})
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, _ = adapter.Update(homeTickRefreshedMsg{
		HasPreview:  true,
		PreviewText: "line 1\nline 2",
	})
	require.True(t, m.Home.hasPreview)
	require.Equal(t, "line 1\nline 2", m.Home.previewText)
}

func TestHandleHomeTickRefreshedClearsPreviewWithoutSession(t *testing.T) {
	m := NewAppModel()
	m.Home.SetPreview("stale", true)
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, _ = adapter.Update(homeTickRefreshedMsg{HasPreview: false})
	require.False(t, m.Home.hasPreview)
	require.Equal(t, "", m.Home.previewText)
}

func TestHandleHomeTickRefreshedShowsCaptureError(t *testing.T) {
	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	_, _ = adapter.Update(homeTickRefreshedMsg{
		HasPreview: true,
		PreviewErr: errors.New("session not found"),
	})
	require.True(t, m.Home.hasPreview)
	require.Contains(t, m.Home.previewText, "capture error")
}
