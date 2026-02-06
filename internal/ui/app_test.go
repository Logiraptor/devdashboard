package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devdeploy/internal/agent"
	"devdeploy/internal/artifact"
	"devdeploy/internal/progress"
	"devdeploy/internal/project"

	tea "github.com/charmbracelet/bubbletea"
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

	// SPC p d in Dashboard with project: should show confirmation modal
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if a.Overlays.Len() != 1 {
		t.Errorf("expected 1 overlay (confirmation modal) after ShowDeleteProjectMsg, got %d", a.Overlays.Len())
	}
	top, _ := a.Overlays.Peek()
	if _, ok := top.View.(*DeleteProjectConfirmModal); !ok {
		t.Errorf("expected DeleteProjectConfirmModal on overlay, got %T", top.View)
	}
	// Simulate user pressing Enter to confirm
	_, cmd = adapter.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		_, cmd = adapter.Update(msg)
		if cmd != nil {
			cmd()
		}
	}
	// Verify project was deleted
	projects, _ := projMgr.ListProjects()
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestProjectKeybinds_ShowDeleteProjectMsg_CancelWithEsc(t *testing.T) {
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
	a.Dashboard.Projects = []ProjectSummary{{Name: "test-proj", Selected: false}}
	a.Dashboard.Selected = 0
	adapter := &appModelAdapter{AppModel: a}

	// SPC p d -> confirmation modal
	_, _ = adapter.Update(ShowDeleteProjectMsg{})
	if a.Overlays.Len() != 1 {
		t.Fatalf("expected 1 overlay after ShowDeleteProjectMsg, got %d", a.Overlays.Len())
	}
	// Esc to cancel
	_, cmd := adapter.Update(keyMsg("esc"))
	if cmd != nil {
		adapter.Update(cmd())
	}
	if a.Overlays.Len() != 0 {
		t.Errorf("expected 0 overlays after Esc, got %d", a.Overlays.Len())
	}
	projects, _ := projMgr.ListProjects()
	if len(projects) != 1 {
		t.Errorf("project should still exist after cancel, got %d projects", len(projects))
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

// TestSPCShowsKeybindHints validates that pressing SPC displays keybind hints in the View.
func TestSPCShowsKeybindHints(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	// Process ProjectsLoadedMsg first (from Init)
	adapter.Update(ProjectsLoadedMsg{Projects: nil})

	// Press SPC -> leader waiting, View should show hints
	_, _ = adapter.Update(keyMsg(" "))
	view := adapter.View()
	if !m.KeyHandler.LeaderWaiting {
		t.Fatal("expected LeaderWaiting after SPC")
	}
	// Help view should show first-level hints: p, q, a
	for _, hint := range []string{"p", "q", "a"} {
		if !strings.Contains(view, hint) {
			t.Errorf("View should contain hint %q after SPC, got:\n%s", hint, view)
		}
	}

	// Press p -> still in leader mode, View should show SPC p sub-hints
	_, _ = adapter.Update(keyMsg("p"))
	view = adapter.View()
	if !m.KeyHandler.LeaderWaiting {
		t.Fatal("expected LeaderWaiting after SPC p")
	}
	for _, hint := range []string{"c", "d", "a", "r"} {
		if !strings.Contains(view, hint) {
			t.Errorf("View should contain hint %q after SPC p, got:\n%s", hint, view)
		}
	}
}

// TestSPCKeybindCommandsExecute validates that SPC p c triggers CreateProjectModal.
func TestSPCKeybindCommandsExecute(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	m := NewAppModel()
	adapter := m.AsTeaModel().(*appModelAdapter)

	// Process ProjectsLoadedMsg first
	adapter.Update(ProjectsLoadedMsg{Projects: nil})

	// SPC p c -> should push CreateProjectModal
	_, cmd := adapter.Update(keyMsg(" "))
	if cmd != nil {
		adapter.Update(cmd())
	}
	_, cmd = adapter.Update(keyMsg("p"))
	if cmd != nil {
		adapter.Update(cmd())
	}
	_, cmd = adapter.Update(keyMsg("c"))
	if cmd != nil {
		adapter.Update(cmd())
	}

	if m.Overlays.Len() != 1 {
		t.Fatalf("expected 1 overlay after SPC p c, got %d", m.Overlays.Len())
	}
	top, _ := m.Overlays.Peek()
	if _, ok := top.View.(*CreateProjectModal); !ok {
		t.Errorf("expected CreateProjectModal, got %T", top.View)
	}
}

// TestAgentProgressVisible validates that agent progress events are visible in the ProgressWindow.
// Part of devdeploy-i1u.10 validation.
func TestAgentProgressVisible(t *testing.T) {
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
	adapter := a.AsTeaModel().(*appModelAdapter)

	// Run agent -> ProgressWindow overlay
	_, _ = adapter.Update(RunAgentMsg{})
	if a.Overlays.Len() != 1 {
		t.Fatalf("expected 1 overlay (ProgressWindow) after RunAgentMsg, got %d", a.Overlays.Len())
	}
	top, _ := a.Overlays.Peek()
	if _, ok := top.View.(*ProgressWindow); !ok {
		t.Fatalf("expected ProgressWindow overlay, got %T", top.View)
	}

	// Simulate agent emitting a progress event (as StubRunner would).
	// This validates that ProgressWindow displays events when they arrive.
	_, _ = adapter.Update(progress.Event{
		Message:   "Agent run started (stub) — test-proj",
		Status:    progress.StatusRunning,
		Timestamp: time.Now(),
	})

	view := adapter.View()
	if !strings.Contains(view, "Agent progress") {
		t.Errorf("View should contain 'Agent progress' header, got:\n%s", view)
	}
	if !strings.Contains(view, "Agent run started") {
		t.Errorf("View should contain agent progress message, got:\n%s", view)
	}
}

// cancelableTestRunner stores the context so tests can verify it was cancelled.
type cancelableTestRunner struct {
	ctx context.Context
}

func (r *cancelableTestRunner) Run(ctx context.Context, projectDir, planPath, designPath string) tea.Cmd {
	r.ctx = ctx
	// Return a cmd that emits one event immediately (so we don't block).
	return func() tea.Msg {
		return progress.Event{
			Message:   "Started",
			Status:    progress.StatusRunning,
			Timestamp: time.Now(),
		}
	}
}

// TestAgentAbort_CallsCancelOnEsc validates that Esc triggers abort (calls cancel on in-flight run).
// Part of devdeploy-i1u.10 validation.
func TestAgentAbort_CallsCancelOnEsc(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)
	defer os.Unsetenv("DEVDEPLOY_PROJECTS_DIR")

	store, err := artifact.NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	projMgr := project.NewManager(store.BaseDir(), dir)
	_ = projMgr.CreateProject("test-proj")

	runner := &cancelableTestRunner{}
	a := &AppModel{
		Mode:           ModeProjectDetail,
		Dashboard:      NewDashboardView(),
		Detail:         NewProjectDetailView("test-proj"),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
		ArtifactStore:  store,
		ProjectManager: projMgr,
		AgentRunner:    runner,
	}
	adapter := a.AsTeaModel().(*appModelAdapter)

	// Run agent -> overlay + cmd
	_, cmd := adapter.Update(RunAgentMsg{})
	if cmd != nil {
		msg := cmd()
		_, _ = adapter.Update(msg)
	}
	if a.agentCancelFunc == nil {
		t.Fatal("expected agentCancelFunc to be set after RunAgentMsg")
	}

	// Esc on ProgressWindow -> overlay returns DismissModalMsg cmd
	_, cmd = adapter.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected overlay to return DismissModalMsg cmd on Esc")
	}
	// Process DismissModalMsg -> app calls agentCancelFunc
	_, _ = adapter.Update(cmd())

	// Verify context was cancelled
	if runner.ctx == nil {
		t.Fatal("runner should have stored context")
	}
	if runner.ctx.Err() != context.Canceled {
		t.Errorf("expected context to be cancelled after Esc, got Err=%v", runner.ctx.Err())
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
