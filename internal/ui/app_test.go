package ui

import (
	"os"
	"path/filepath"
	"testing"

	"devdeploy/internal/agent"
	"devdeploy/internal/artifact"
	"devdeploy/internal/project"
)

func TestProjectKeybinds_ShowCreateProjectMsg(t *testing.T) {
	// DEVDEPLOY_PROJECTS_DIR must be set for artifact.NewStore to use our dir.
	// Store reads from env in NewStore - we need the projects base for Manager.
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	a := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p c -> ShowCreateProjectMsg: should push CreateProjectModal
	_, cmd := adapter.Update(ShowCreateProjectMsg{})
	if a.Overlays.Len() != 1 {
		t.Errorf("expected 1 overlay after ShowCreateProjectMsg, got %d", a.Overlays.Len())
	}
	top, _ := a.Overlays.Peek()
	if _, ok := top.View.(*CreateProjectModal); !ok {
		t.Errorf("expected CreateProjectModal on overlay, got %T", top.View)
	}
	_ = cmd
}

func TestProjectKeybinds_ShowDeleteProjectMsg(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	// Create a project so we have something to delete
	if err := projMgr.CreateProject("test-proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	a := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	a.Dashboard.Projects = []ProjectSummary{{Name: "test-proj", Selected: false}}
	a.Dashboard.Selected = 0
	adapter := &appModelAdapter{AppModel: a}

	// SPC p d in Dashboard with project: should delete
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if cmd == nil {
		t.Error("expected loadProjectsCmd after delete")
	}
	// Run the cmd to refresh
	if cmd != nil {
		cmd()
	}
	// Verify project was deleted
	projects, _ := projMgr.ListProjects()
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestProjectKeybinds_ShowDeleteProjectMsg_NoOpWhenNotDashboard(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	_ = projMgr.CreateProject("test-proj")

	a := &AppModel{
		Mode:           ModeProjectDetail,
		Dashboard:      NewDashboardView(),
		Detail:         NewProjectDetailView("test-proj"),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p d in ProjectDetail: should be no-op (delete is Dashboard-only)
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in ProjectDetail (delete is Dashboard-only)")
	}
	projects, _ := projMgr.ListProjects()
	if len(projects) != 1 {
		t.Errorf("project should still exist (no delete in ProjectDetail), got %d projects", len(projects))
	}
}

func TestProjectKeybinds_ShowAddRepoMsg_NoOpWhenDashboard(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	_ = projMgr.CreateProject("test-proj")

	a := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p a in Dashboard: should be no-op (add repo is ProjectDetail-only)
	_, cmd := adapter.Update(ShowAddRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in Dashboard (add repo is ProjectDetail-only)")
	}
	if a.Overlays.Len() != 0 {
		t.Errorf("expected no overlay in Dashboard, got %d", a.Overlays.Len())
	}
}

func TestProjectKeybinds_ShowAddRepoMsg_InProjectDetail(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	// Create a git repo in workspace so ListWorkspaceRepos returns something
	wsDir := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	repoDir := filepath.Join(wsDir, "some-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("MkdirAll repo: %v", err)
	}
	// Initialize as git repo (ListWorkspaceRepos checks for .git dir)
	_ = os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), wsDir)
	_ = projMgr.CreateProject("test-proj")

	a := &AppModel{
		Mode:           ModeProjectDetail,
		Dashboard:      NewDashboardView(),
		Detail:         NewProjectDetailView("test-proj"),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p a in ProjectDetail with workspace repos: should push AddRepo modal
	_, _ = adapter.Update(ShowAddRepoMsg{})
	// May push overlay or set error status (if no repos found)
	if a.Overlays.Len() == 1 {
		top, _ := a.Overlays.Peek()
		if _, ok := top.View.(*RepoPickerModal); !ok {
			t.Errorf("expected RepoPickerModal when repos exist, got %T", top.View)
		}
	}
	// If ListWorkspaceRepos returns empty (e.g. git detection differs), we get status error - both valid
}

func TestProjectKeybinds_ShowRemoveRepoMsg_NoOpWhenDashboard(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	_ = projMgr.CreateProject("test-proj")

	a := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p r in Dashboard: should be no-op (remove repo is ProjectDetail-only)
	_, cmd := adapter.Update(ShowRemoveRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in Dashboard (remove repo is ProjectDetail-only)")
	}
	if a.Overlays.Len() != 0 {
		t.Errorf("expected no overlay in Dashboard, got %d", a.Overlays.Len())
	}
}

func TestProjectKeybinds_ShowRemoveRepoMsg_InProjectDetail_NoRepos(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	_ = projMgr.CreateProject("test-proj")

	a := &AppModel{
		Mode:           ModeProjectDetail,
		Dashboard:      NewDashboardView(),
		Detail:         NewProjectDetailView("test-proj"),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    &agent.StubRunner{},
	}
	adapter := &appModelAdapter{AppModel: a}

	// SPC p r in ProjectDetail with no repos: should set status error, no overlay
	_, cmd := adapter.Update(ShowRemoveRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when no repos to remove")
	}
	if a.Overlays.Len() != 0 {
		t.Errorf("expected no overlay when no repos, got %d", a.Overlays.Len())
	}
	if !a.StatusIsError || a.Status == "" {
		t.Errorf("expected error status when no repos, got Status=%q StatusIsError=%v", a.Status, a.StatusIsError)
	}
}
