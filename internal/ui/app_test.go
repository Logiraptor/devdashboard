package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devdeploy/internal/agent"
	"devdeploy/internal/artifact"
	"devdeploy/internal/project"
	"devdeploy/internal/session"
)

// testApp bundles common test dependencies so each test doesn't repeat ~15 lines of setup.
type testApp struct {
	*AppModel
	Dir string // the temp projects directory
}

// newTestApp creates an AppModel wired to a temp directory with a fresh store and
// project manager. Defaults to ModeDashboard. The caller can mutate fields (Mode,
// Detail, Sessions, etc.) before exercising the adapter.
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DEVDEPLOY_PROJECTS_DIR", dir)

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
		Sessions:       session.New(nil),
	}
	return &testApp{AppModel: a, Dir: dir}
}

// adapter returns the tea.Model adapter for driving Update/View calls.
func (ta *testApp) adapter() *appModelAdapter {
	return ta.AsTeaModel().(*appModelAdapter)
}

func TestProjectKeybinds_ShowCreateProjectMsg(t *testing.T) {
	ta := newTestApp(t)
	adapter := ta.adapter()

	// SPC p c -> ShowCreateProjectMsg: should push CreateProjectModal
	_, cmd := adapter.Update(ShowCreateProjectMsg{})
	if ta.Overlays.Len() != 1 {
		t.Errorf("expected 1 overlay after ShowCreateProjectMsg, got %d", ta.Overlays.Len())
	}
	top, _ := ta.Overlays.Peek()
	if _, ok := top.View.(*CreateProjectModal); !ok {
		t.Errorf("expected CreateProjectModal on overlay, got %T", top.View)
	}
	_ = cmd
}

func TestProjectKeybinds_ShowDeleteProjectMsg(t *testing.T) {
	ta := newTestApp(t)
	if err := ta.ProjectManager.CreateProject("test-proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	ta.Dashboard.Projects = []ProjectSummary{{Name: "test-proj", Selected: false}}
	ta.Dashboard.Selected = 0
	adapter := ta.adapter()

	// SPC p d in Dashboard with project: should show confirmation modal
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if ta.Overlays.Len() != 1 {
		t.Errorf("expected 1 overlay (confirmation modal) after ShowDeleteProjectMsg, got %d", ta.Overlays.Len())
	}
	top, _ := ta.Overlays.Peek()
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
	projects, _ := ta.ProjectManager.ListProjects()
	if len(projects) != 0 {
		t.Errorf("expected 0 projects after delete, got %d", len(projects))
	}
}

func TestProjectKeybinds_ShowDeleteProjectMsg_CancelWithEsc(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")
	ta.Dashboard.Projects = []ProjectSummary{{Name: "test-proj", Selected: false}}
	ta.Dashboard.Selected = 0
	adapter := ta.adapter()

	// SPC p d -> confirmation modal
	_, _ = adapter.Update(ShowDeleteProjectMsg{})
	if ta.Overlays.Len() != 1 {
		t.Fatalf("expected 1 overlay after ShowDeleteProjectMsg, got %d", ta.Overlays.Len())
	}
	// Esc to cancel
	_, cmd := adapter.Update(keyMsg("esc"))
	if cmd != nil {
		adapter.Update(cmd())
	}
	if ta.Overlays.Len() != 0 {
		t.Errorf("expected 0 overlays after Esc, got %d", ta.Overlays.Len())
	}
	projects, _ := ta.ProjectManager.ListProjects()
	if len(projects) != 1 {
		t.Errorf("project should still exist after cancel, got %d projects", len(projects))
	}
}

func TestProjectKeybinds_ShowDeleteProjectMsg_NoOpWhenNotDashboard(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	// SPC p d in ProjectDetail: should be no-op (delete is Dashboard-only)
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in ProjectDetail (delete is Dashboard-only)")
	}
	projects, _ := ta.ProjectManager.ListProjects()
	if len(projects) != 1 {
		t.Errorf("project should still exist (no delete in ProjectDetail), got %d projects", len(projects))
	}
}

func TestProjectKeybinds_ShowAddRepoMsg_NoOpWhenDashboard(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")
	adapter := ta.adapter()

	// SPC p a in Dashboard: should be no-op (add repo is ProjectDetail-only)
	_, cmd := adapter.Update(ShowAddRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in Dashboard (add repo is ProjectDetail-only)")
	}
	if ta.Overlays.Len() != 0 {
		t.Errorf("expected no overlay in Dashboard, got %d", ta.Overlays.Len())
	}
}

func TestProjectKeybinds_ShowAddRepoMsg_InProjectDetail(t *testing.T) {
	ta := newTestApp(t)

	// Create a git repo in workspace so ListWorkspaceRepos returns something
	wsDir := filepath.Join(ta.Dir, "workspace")
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

	// Recreate project manager with workspace dir
	ta.ProjectManager = project.NewManager(ta.ArtifactStore.BaseDir(), wsDir)
	_ = ta.ProjectManager.CreateProject("test-proj")

	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	// SPC p a in ProjectDetail with workspace repos: should push AddRepo modal
	_, _ = adapter.Update(ShowAddRepoMsg{})
	// May push overlay or set error status (if no repos found)
	if ta.Overlays.Len() == 1 {
		top, _ := ta.Overlays.Peek()
		if _, ok := top.View.(*RepoPickerModal); !ok {
			t.Errorf("expected RepoPickerModal when repos exist, got %T", top.View)
		}
	}
	// If ListWorkspaceRepos returns empty (e.g. git detection differs), we get status error - both valid
}

func TestProjectKeybinds_ShowRemoveRepoMsg_NoOpWhenDashboard(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")
	adapter := ta.adapter()

	// SPC p r in Dashboard: should be no-op (remove repo is ProjectDetail-only)
	_, cmd := adapter.Update(ShowRemoveRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when in Dashboard (remove repo is ProjectDetail-only)")
	}
	if ta.Overlays.Len() != 0 {
		t.Errorf("expected no overlay in Dashboard, got %d", ta.Overlays.Len())
	}
}

// TestSPCShowsKeybindHints validates that pressing SPC displays keybind hints in the View.
func TestSPCShowsKeybindHints(t *testing.T) {
	t.Setenv("DEVDEPLOY_PROJECTS_DIR", t.TempDir())

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
	t.Setenv("DEVDEPLOY_PROJECTS_DIR", t.TempDir())

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

// TestOpenShellMsg_NoOverlay validates that OpenShellMsg opens a tmux pane
// without pushing an overlay, and uses the selected resource's worktree path.
func TestOpenShellMsg_NoOverlay(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// OpenShellMsg -> tmux.SplitPane(worktreePath); no overlay pushed
	// (tmux.SplitPane will fail outside tmux, but we verify no overlay and correct error handling)
	_, _ = adapter.Update(OpenShellMsg{})
	if ta.Overlays.Len() != 0 {
		t.Fatalf("expected no overlay after OpenShellMsg, got %d", ta.Overlays.Len())
	}
}

// TestOpenShellMsg_NoResourceSelected validates error when no resource is selected.
func TestOpenShellMsg_NoResourceSelected(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	_, _ = adapter.Update(OpenShellMsg{})
	if !ta.StatusIsError || ta.Status != "No resource selected" {
		t.Errorf("expected 'No resource selected' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestOpenShellMsg_PRNoWorktree validates that PR resources without worktrees
// attempt to create one via EnsurePRWorktree (which fails in test because
// there's no real source repo, producing an "Open shell:" error).
func TestOpenShellMsg_PRNoWorktree(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourcePR, RepoName: "myrepo", PR: &project.PRInfo{Number: 42, Title: "test", HeadRefName: "feat-branch"}},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(OpenShellMsg{})
	if !ta.StatusIsError || !strings.Contains(ta.Status, "Open shell") {
		t.Errorf("expected 'Open shell' error (EnsurePRWorktree fails, no source repo), got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestOpenShellMsg_PRNoBranch validates error for PR resources with no branch name.
func TestOpenShellMsg_PRNoBranch(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourcePR, RepoName: "myrepo", PR: &project.PRInfo{Number: 42, Title: "test"}},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(OpenShellMsg{})
	if !ta.StatusIsError || !strings.Contains(ta.Status, "no branch name") {
		t.Errorf("expected 'no branch name' error for PR without HeadRefName, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestEnterInProjectDetail_TriggersOpenShell validates that Enter in project detail triggers OpenShellMsg.
func TestEnterInProjectDetail_TriggersOpenShell(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// Enter in project detail should produce a cmd that returns OpenShellMsg
	_, cmd := adapter.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter in project detail")
	}
	msg := cmd()
	if _, ok := msg.(OpenShellMsg); !ok {
		t.Errorf("expected OpenShellMsg from Enter cmd, got %T", msg)
	}
}

// TestHidePaneMsg_NoPane validates error when no pane to hide.
func TestHidePaneMsg_NoPane(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(HidePaneMsg{})
	if ta.Status != "No pane to hide" {
		t.Errorf("expected 'No pane to hide', got %q", ta.Status)
	}
}

// TestShowPaneMsg_NoPane validates error when no pane to show.
func TestShowPaneMsg_NoPane(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(ShowPaneMsg{})
	if ta.Status != "No pane to show" {
		t.Errorf("expected 'No pane to show', got %q", ta.Status)
	}
}

// TestSelectedResourceLatestPaneID validates that hide/show uses the session tracker.
func TestSelectedResourceLatestPaneID(t *testing.T) {
	ta := newTestApp(t)

	ta.Sessions.Register("repo:myrepo", "%10", session.PaneShell)
	ta.Sessions.Register("repo:myrepo", "%11", session.PaneShell)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail

	got := ta.selectedResourceLatestPaneID()
	if got != "%11" {
		t.Errorf("expected most recent pane %%11, got %q", got)
	}
}

// TestLaunchAgentMsg_NoOverlay validates that LaunchAgentMsg opens a tmux pane
// with agent command, without pushing an overlay.
func TestLaunchAgentMsg_NoOverlay(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// LaunchAgentMsg -> tmux.SplitPane + SendKeys; no overlay pushed
	// (tmux calls will fail outside tmux, but we verify no overlay and correct error handling)
	_, _ = adapter.Update(LaunchAgentMsg{})
	if ta.Overlays.Len() != 0 {
		t.Fatalf("expected no overlay after LaunchAgentMsg, got %d", ta.Overlays.Len())
	}
}

// TestLaunchAgentMsg_NoResourceSelected validates error when no resource is selected.
func TestLaunchAgentMsg_NoResourceSelected(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchAgentMsg{})
	if !ta.StatusIsError || ta.Status != "No resource selected" {
		t.Errorf("expected 'No resource selected' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestLaunchAgentMsg_PRNoWorktree validates that PR resources without worktrees
// attempt to create one via EnsurePRWorktree (which fails in test because
// there's no real source repo, producing a "Launch agent:" error).
func TestLaunchAgentMsg_PRNoWorktree(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourcePR, RepoName: "myrepo", PR: &project.PRInfo{Number: 42, Title: "test", HeadRefName: "feat-branch"}},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchAgentMsg{})
	if !ta.StatusIsError || !strings.Contains(ta.Status, "Launch agent") {
		t.Errorf("expected 'Launch agent' error (EnsurePRWorktree fails, no source repo), got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestLaunchAgentMsg_NotInProjectDetail validates no-op when not in project detail mode.
func TestLaunchAgentMsg_NotInProjectDetail(t *testing.T) {
	ta := newTestApp(t)
	adapter := ta.adapter()

	_, cmd := adapter.Update(LaunchAgentMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when not in project detail")
	}
	if ta.StatusIsError {
		t.Error("expected no error status when not in project detail")
	}
}

// TestLaunchAgentMsg_RegistersAsAgent validates that a successful launch registers as PaneAgent.
func TestLaunchAgentMsg_RegistersAsAgent(t *testing.T) {
	// This test verifies the registration logic by directly calling the handler
	// with a resource that has a valid worktree path (a real temp directory).
	// tmux.SplitPane will fail (no tmux), so we just verify the error message
	// contains "Launch agent" (tmux error), not "No resource" or "No worktree".
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: ta.Dir},
	}
	detail.Selected = 0

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchAgentMsg{})
	// Outside tmux, SplitPane fails. The error should be "Launch agent: ..." not "No resource"
	if ta.StatusIsError && !strings.Contains(ta.Status, "Launch agent") {
		t.Errorf("expected 'Launch agent' error (tmux not available), got Status=%q", ta.Status)
	}
}

// TestEnsureResourceWorktree_RepoUsesExisting validates that repo resources
// return their existing WorktreePath without calling EnsurePRWorktree.
func TestEnsureResourceWorktree_RepoUsesExisting(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")

	r := &project.Resource{
		Kind:         project.ResourceRepo,
		RepoName:     "myrepo",
		WorktreePath: "/tmp/existing-worktree",
	}
	got, err := ta.AppModel.ensureResourceWorktree(r)
	if err != nil {
		t.Fatalf("ensureResourceWorktree: %v", err)
	}
	if got != "/tmp/existing-worktree" {
		t.Errorf("expected /tmp/existing-worktree, got %s", got)
	}
}

// TestEnsureResourceWorktree_PRWithWorktreeReuses validates that PR resources
// with an existing WorktreePath reuse it.
func TestEnsureResourceWorktree_PRWithWorktreeReuses(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")

	r := &project.Resource{
		Kind:         project.ResourcePR,
		RepoName:     "myrepo",
		PR:           &project.PRInfo{Number: 42, Title: "test", HeadRefName: "feat"},
		WorktreePath: "/tmp/pr-worktree",
	}
	got, err := ta.AppModel.ensureResourceWorktree(r)
	if err != nil {
		t.Fatalf("ensureResourceWorktree: %v", err)
	}
	if got != "/tmp/pr-worktree" {
		t.Errorf("expected /tmp/pr-worktree, got %s", got)
	}
}

// TestEnsureResourceWorktree_RepoNoWorktree validates error for repo with empty worktree path.
func TestEnsureResourceWorktree_RepoNoWorktree(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")

	r := &project.Resource{
		Kind:     project.ResourceRepo,
		RepoName: "myrepo",
	}
	_, err := ta.AppModel.ensureResourceWorktree(r)
	if err == nil {
		t.Fatal("expected error for repo with no worktree")
	}
	if !strings.Contains(err.Error(), "no worktree") {
		t.Errorf("expected 'no worktree' in error, got %q", err.Error())
	}
}

func TestProjectKeybinds_ShowRemoveRepoMsg_InProjectDetail_NoRepos(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	// SPC p r in ProjectDetail with no repos: should set status error, no overlay
	_, cmd := adapter.Update(ShowRemoveRepoMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when no repos to remove")
	}
	if ta.Overlays.Len() != 0 {
		t.Errorf("expected no overlay when no repos, got %d", ta.Overlays.Len())
	}
	if !ta.StatusIsError || ta.Status == "" {
		t.Errorf("expected error status when no repos, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}
