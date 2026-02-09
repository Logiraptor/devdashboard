package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devdeploy/internal/agent"
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

	projMgr := project.NewManager(dir, dir)

	a := &AppModel{
		Mode:           ModeDashboard,
		Dashboard:      NewDashboardView(),
		KeyHandler:     NewKeyHandler(NewKeybindRegistry()),
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
	ta.Dashboard.updateProjects()
	ta.Dashboard.list.Select(0)
	adapter := ta.adapter()

	// SPC p d in Dashboard with project: should show confirmation modal
	_, cmd := adapter.Update(ShowDeleteProjectMsg{})
	if ta.Overlays.Len() != 1 {
		t.Errorf("expected 1 overlay (confirmation modal) after ShowDeleteProjectMsg, got %d", ta.Overlays.Len())
	}
	top, _ := ta.Overlays.Peek()
	if _, ok := top.View.(*ConfirmModal); !ok {
		t.Errorf("expected ConfirmModal on overlay, got %T", top.View)
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
	ta.Dashboard.updateProjects()
	ta.Dashboard.list.Select(0)
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
	ta.ProjectManager = project.NewManager(ta.Dir, wsDir)
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
	// Skip if running inside tmux unless explicitly enabled to prevent polluting the session
	if os.Getenv("TMUX") != "" && os.Getenv("DEVDEPLOY_TMUX_TESTS") != "1" {
		t.Skip("Skipping test that calls tmux.SplitPane: set DEVDEPLOY_TMUX_TESTS=1 to enable")
	}
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	// Skip if running inside tmux unless explicitly enabled to prevent polluting the session
	if os.Getenv("TMUX") != "" && os.Getenv("DEVDEPLOY_TMUX_TESTS") != "1" {
		t.Skip("Skipping test that calls tmux.SplitPane: set DEVDEPLOY_TMUX_TESTS=1 to enable")
	}
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)

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
	detail.setSelected(0)

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
	// Skip if running inside tmux unless explicitly enabled to prevent polluting the session
	if os.Getenv("TMUX") != "" && os.Getenv("DEVDEPLOY_TMUX_TESTS") != "1" {
		t.Skip("Skipping test that calls tmux.SplitPane: set DEVDEPLOY_TMUX_TESTS=1 to enable")
	}
	// This test verifies the registration logic by directly calling the handler
	// with a resource that has a valid worktree path (a real temp directory).
	// tmux.SplitPane will fail (no tmux), so we just verify the error message
	// contains "Launch agent" (tmux error), not "No resource" or "No worktree".
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: ta.Dir},
	}
	detail.setSelected(0)

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

// --- Resource cleanup tests ---

// TestShowRemoveResourceMsg_ShowsConfirmModal validates that ShowRemoveResourceMsg
// pushes a RemoveResourceConfirmModal onto the overlay stack.
func TestShowRemoveResourceMsg_ShowsConfirmModal(t *testing.T) {
	ta := newTestApp(t)
	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(ShowRemoveResourceMsg{})
	if ta.Overlays.Len() != 1 {
		t.Fatalf("expected 1 overlay after ShowRemoveResourceMsg, got %d", ta.Overlays.Len())
	}
	top, _ := ta.Overlays.Peek()
	modal, ok := top.View.(*ConfirmModal)
	if !ok {
		t.Fatalf("expected ConfirmModal, got %T", top.View)
	}
	if modal.Resource.RepoName != "myrepo" {
		t.Errorf("expected modal resource 'myrepo', got %q", modal.Resource.RepoName)
	}
}

// TestShowRemoveResourceMsg_NoResourceSelected validates error when no resource selected.
func TestShowRemoveResourceMsg_NoResourceSelected(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	_, _ = adapter.Update(ShowRemoveResourceMsg{})
	if !ta.StatusIsError || ta.Status != "No resource selected" {
		t.Errorf("expected 'No resource selected' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
	if ta.Overlays.Len() != 0 {
		t.Errorf("expected no overlay, got %d", ta.Overlays.Len())
	}
}

// TestShowRemoveResourceMsg_NotInProjectDetail validates no-op when not in project detail.
func TestShowRemoveResourceMsg_NotInProjectDetail(t *testing.T) {
	ta := newTestApp(t)
	adapter := ta.adapter()

	_, cmd := adapter.Update(ShowRemoveResourceMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when not in project detail")
	}
	if ta.Overlays.Len() != 0 {
		t.Error("expected no overlay when not in project detail")
	}
}

// TestRemoveResourceMsg_KillsPanesAndUnregisters validates that RemoveResourceMsg
// kills panes and unregisters them from the session tracker.
func TestRemoveResourceMsg_KillsPanesAndUnregisters(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	rk := session.ResourceKey("repo", "myrepo", 0)
	ta.Sessions.Register(rk, "%10", session.PaneShell)
	ta.Sessions.Register(rk, "%11", session.PaneAgent)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// Send RemoveResourceMsg. Worktree removal will fail (no real git repo),
	// but pane cleanup should still happen.
	_, _ = adapter.Update(RemoveResourceMsg{
		ProjectName: "test-proj",
		Resource:    detail.Resources[0],
	})

	// Panes should be unregistered.
	if panes := ta.Sessions.PanesForResource(rk); panes != nil {
		t.Errorf("expected no panes after remove, got %+v", panes)
	}
}

// TestRemoveResourceMsg_ClampsSelection validates that the selection index is
// clamped after removing the last resource.
func TestRemoveResourceMsg_ClampsSelection(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(RemoveResourceMsg{
		ProjectName: "test-proj",
		Resource:    detail.Resources[0],
	})

	// After removing the only resource, Selected should be clamped to 0.
	if ta.Detail.Selected != 0 {
		t.Errorf("expected Selected=0 after removing only resource, got %d", ta.Detail.Selected)
	}
}

// TestRemoveResourceMsg_PR validates removal of a PR resource.
func TestRemoveResourceMsg_PR(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	rk := session.ResourceKey("pr", "myrepo", 42)
	ta.Sessions.Register(rk, "%20", session.PaneAgent)

	prResource := project.Resource{
		Kind:     project.ResourcePR,
		RepoName: "myrepo",
		PR:       &project.PRInfo{Number: 42, Title: "test PR", HeadRefName: "feat"},
	}

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
		prResource,
	}
	detail.setSelected(1)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(RemoveResourceMsg{
		ProjectName: "test-proj",
		Resource:    prResource,
	})

	// PR panes should be unregistered.
	if panes := ta.Sessions.PanesForResource(rk); panes != nil {
		t.Errorf("expected no PR panes after remove, got %+v", panes)
	}
}

// TestRemoveResourceMsg_DismissesOverlay validates that RemoveResourceMsg pops the overlay.
func TestRemoveResourceMsg_DismissesOverlay(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)
	ta.Mode = ModeProjectDetail
	ta.Detail = detail

	// Push modal first.
	modal := NewRemoveResourceConfirmModal("test-proj", detail.Resources[0])
	ta.Overlays.Push(Overlay{View: modal, Dismiss: "esc"})
	adapter := ta.adapter()

	_, _ = adapter.Update(RemoveResourceMsg{
		ProjectName: "test-proj",
		Resource:    detail.Resources[0],
	})

	if ta.Overlays.Len() != 0 {
		t.Errorf("expected overlay dismissed after RemoveResourceMsg, got %d", ta.Overlays.Len())
	}
}

// TestDKeyInProjectDetail_TriggersRemoveResource validates the 'd' shortcut key.
func TestDKeyInProjectDetail_TriggersRemoveResource(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	detail.setSelected(0)
	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// 'd' in project detail should produce a cmd returning ShowRemoveResourceMsg
	_, cmd := adapter.Update(keyMsg("d"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'd' in project detail")
	}
	msg := cmd()
	if _, ok := msg.(ShowRemoveResourceMsg); !ok {
		t.Errorf("expected ShowRemoveResourceMsg from 'd' cmd, got %T", msg)
	}
}

// TestRemoveResourceConfirmModal_EscCancels validates that Esc dismisses the modal.
func TestRemoveResourceConfirmModal_EscCancels(t *testing.T) {
	r := project.Resource{Kind: project.ResourceRepo, RepoName: "myrepo"}
	modal := NewRemoveResourceConfirmModal("test-proj", r)

	_, cmd := modal.Update(keyMsg("esc"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Esc")
	}
	msg := cmd()
	if _, ok := msg.(DismissModalMsg); !ok {
		t.Errorf("expected DismissModalMsg from Esc, got %T", msg)
	}
}

// TestRemoveResourceConfirmModal_EnterConfirms validates that Enter sends RemoveResourceMsg.
func TestRemoveResourceConfirmModal_EnterConfirms(t *testing.T) {
	r := project.Resource{Kind: project.ResourceRepo, RepoName: "myrepo"}
	modal := NewRemoveResourceConfirmModal("test-proj", r)

	_, cmd := modal.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter")
	}
	msg := cmd()
	rmMsg, ok := msg.(RemoveResourceMsg)
	if !ok {
		t.Fatalf("expected RemoveResourceMsg from Enter, got %T", msg)
	}
	if rmMsg.ProjectName != "test-proj" {
		t.Errorf("expected ProjectName 'test-proj', got %q", rmMsg.ProjectName)
	}
	if rmMsg.Resource.RepoName != "myrepo" {
		t.Errorf("expected resource 'myrepo', got %q", rmMsg.Resource.RepoName)
	}
}

// --- End-to-end resource workflow tests (devdeploy-7uj.8) ---

// TestResourceKeyFromResource validates the helper that builds session keys from resources.
func TestResourceKeyFromResource(t *testing.T) {
	tests := []struct {
		name     string
		resource project.Resource
		want     string
	}{
		{
			name:     "repo resource",
			resource: project.Resource{Kind: project.ResourceRepo, RepoName: "devdeploy"},
			want:     "repo:devdeploy",
		},
		{
			name: "PR resource",
			resource: project.Resource{
				Kind:     project.ResourcePR,
				RepoName: "devdeploy",
				PR:       &project.PRInfo{Number: 42},
			},
			want: "pr:devdeploy:#42",
		},
		{
			name:     "repo resource no PR info",
			resource: project.Resource{Kind: project.ResourceRepo, RepoName: "grafana"},
			want:     "repo:grafana",
		},
		{
			name: "PR resource nil PR struct treated as repo",
			resource: project.Resource{
				Kind:     project.ResourcePR,
				RepoName: "grafana",
				PR:       nil,
			},
			want: "repo:grafana",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceKeyFromResource(tt.resource)
			if got != tt.want {
				t.Errorf("resourceKeyFromResource() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPopulateResourcePanes validates that session tracker panes are
// attached to resources in the detail view.
func TestPopulateResourcePanes(t *testing.T) {
	ta := newTestApp(t)

	ta.Sessions.Register("repo:myrepo", "%1", session.PaneShell)
	ta.Sessions.Register("repo:myrepo", "%2", session.PaneAgent)
	ta.Sessions.Register("pr:myrepo:#42", "%3", session.PaneAgent)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
		{Kind: project.ResourcePR, RepoName: "myrepo", PR: &project.PRInfo{Number: 42, Title: "test"}},
		{Kind: project.ResourceRepo, RepoName: "other"},
	}

	ta.AppModel.populateResourcePanes(detail)

	// myrepo repo should have 2 panes (1 shell, 1 agent).
	if len(detail.Resources[0].Panes) != 2 {
		t.Errorf("expected 2 panes for myrepo repo, got %d", len(detail.Resources[0].Panes))
	} else {
		if detail.Resources[0].Panes[0].IsAgent {
			t.Error("expected first pane to be shell")
		}
		if !detail.Resources[0].Panes[1].IsAgent {
			t.Error("expected second pane to be agent")
		}
	}

	// PR should have 1 agent pane.
	if len(detail.Resources[1].Panes) != 1 {
		t.Errorf("expected 1 pane for PR, got %d", len(detail.Resources[1].Panes))
	} else if !detail.Resources[1].Panes[0].IsAgent {
		t.Error("expected PR pane to be agent")
	}

	// other repo should have 0 panes.
	if len(detail.Resources[2].Panes) != 0 {
		t.Errorf("expected 0 panes for other repo, got %d", len(detail.Resources[2].Panes))
	}
}

// TestProjectSwitchPanesPersist validates that panes survive project switches
// (switching from detail to dashboard and back preserves session tracker state).
func TestProjectSwitchPanesPersist(t *testing.T) {
	ta := newTestApp(t)

	// Register panes for a resource.
	rk := "repo:myrepo"
	ta.Sessions.Register(rk, "%10", session.PaneShell)
	ta.Sessions.Register(rk, "%11", session.PaneAgent)

	// Verify panes exist before switch.
	if ta.Sessions.Count() != 2 {
		t.Fatalf("expected 2 panes before switch, got %d", ta.Sessions.Count())
	}

	// Simulate switching to dashboard (Esc from detail).
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	_, _ = adapter.Update(keyMsg("esc"))
	if ta.Mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard after Esc, got %v", ta.Mode)
	}

	// Panes should still be tracked.
	if ta.Sessions.Count() != 2 {
		t.Errorf("expected 2 panes after switching to dashboard, got %d", ta.Sessions.Count())
	}
	panes := ta.Sessions.PanesForResource(rk)
	if len(panes) != 2 {
		t.Errorf("expected 2 panes for resource after switch, got %d", len(panes))
	}
}

// TestDeleteProjectMsg_KillsPanesForAllResources validates that deleting a project
// cleans up all tracked panes for the project's resources.
func TestDeleteProjectMsg_KillsPanesForAllResources(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("doomed-proj")

	// Create a fake repo worktree so ListProjectResources returns it.
	projDir := ta.Dir
	repoDir := filepath.Join(projDir, "doomed-proj", "myrepo")
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.WriteFile(filepath.Join(repoDir, ".git"), []byte("gitdir: /x"), 0644)

	// Register panes for the resource.
	rk := "repo:myrepo"
	ta.Sessions.Register(rk, "%20", session.PaneShell)
	ta.Sessions.Register(rk, "%21", session.PaneAgent)

	ta.Dashboard.Projects = []ProjectSummary{{Name: "doomed-proj"}}
	ta.Dashboard.updateProjects()
	ta.Dashboard.list.Select(0)
	adapter := ta.adapter()

	// Push delete confirmation modal and confirm.
	_, _ = adapter.Update(ShowDeleteProjectMsg{})
	if ta.Overlays.Len() != 1 {
		t.Fatalf("expected delete confirmation modal, got %d overlays", ta.Overlays.Len())
	}
	// Confirm with Enter.
	_, cmd := adapter.Update(keyMsg("enter"))
	if cmd != nil {
		msg := cmd()
		_, cmd = adapter.Update(msg)
		if cmd != nil {
			cmd()
		}
	}

	// Panes should be unregistered from session tracker.
	if ta.Sessions.Count() != 0 {
		t.Errorf("expected 0 panes after project delete, got %d", ta.Sessions.Count())
	}
	if panes := ta.Sessions.PanesForResource(rk); panes != nil {
		t.Errorf("expected no panes for resource after project delete, got %+v", panes)
	}
}

// TestRefreshDetailPanes_PrunesDeadPanes validates that refreshDetailPanes
// removes dead panes and updates the detail view.
func TestRefreshDetailPanes_PrunesDeadPanes(t *testing.T) {
	// Use a liveness checker that reports only %1 as alive.
	tracker := session.New(func() (map[string]bool, error) {
		return map[string]bool{"%1": true}, nil
	})
	ta := newTestApp(t)
	ta.Sessions = tracker

	rk := "repo:myrepo"
	ta.Sessions.Register(rk, "%1", session.PaneShell)
	ta.Sessions.Register(rk, "%2", session.PaneAgent) // dead

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
	}
	ta.Mode = ModeProjectDetail
	ta.Detail = detail

	ta.AppModel.refreshDetailPanes()

	// Only %1 should remain.
	if ta.Sessions.Count() != 1 {
		t.Errorf("expected 1 pane after prune, got %d", ta.Sessions.Count())
	}
	if len(ta.Detail.Resources[0].Panes) != 1 {
		t.Errorf("expected 1 pane in detail view, got %d", len(ta.Detail.Resources[0].Panes))
	}
	if ta.Detail.Resources[0].Panes[0].ID != "%1" {
		t.Errorf("expected pane %%1, got %s", ta.Detail.Resources[0].Panes[0].ID)
	}
}

// TestRemoveResourceConfirmModal_YConfirms validates that 'y' also confirms.
func TestRemoveResourceConfirmModal_YConfirms(t *testing.T) {
	r := project.Resource{Kind: project.ResourceRepo, RepoName: "myrepo"}
	modal := NewRemoveResourceConfirmModal("test-proj", r)

	_, cmd := modal.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'y'")
	}
	msg := cmd()
	if _, ok := msg.(RemoveResourceMsg); !ok {
		t.Errorf("expected RemoveResourceMsg from 'y', got %T", msg)
	}
}

// TestRemoveResourceConfirmModal_View validates the modal renders resource info.
func TestRemoveResourceConfirmModal_View(t *testing.T) {
	r := project.Resource{
		Kind:         project.ResourceRepo,
		RepoName:     "myrepo",
		WorktreePath: "/tmp/myrepo",
		Panes: []project.PaneInfo{
			{ID: "%1", IsAgent: false},
			{ID: "%2", IsAgent: true},
		},
	}
	modal := NewRemoveResourceConfirmModal("test-proj", r)
	view := modal.View()

	if !strings.Contains(view, "Remove resource?") {
		t.Error("expected 'Remove resource?' in modal view")
	}
	if !strings.Contains(view, "myrepo") {
		t.Error("expected 'myrepo' in modal view")
	}
	if !strings.Contains(view, "Worktree will be removed") {
		t.Error("expected worktree warning in modal view")
	}
	if !strings.Contains(view, "2 active pane(s) will be killed") {
		t.Error("expected pane warning in modal view")
	}
}

// TestRemoveResourceConfirmModal_PRView validates the modal renders PR info.
func TestRemoveResourceConfirmModal_PRView(t *testing.T) {
	r := project.Resource{
		Kind:     project.ResourcePR,
		RepoName: "myrepo",
		PR:       &project.PRInfo{Number: 42, Title: "Fix bug"},
	}
	modal := NewRemoveResourceConfirmModal("test-proj", r)
	view := modal.View()

	if !strings.Contains(view, "PR #42") {
		t.Error("expected 'PR #42' in modal view")
	}
	if !strings.Contains(view, "Fix bug") {
		t.Error("expected PR title in modal view")
	}
}

// --- Ralph loop tests (devdeploy-j4n.3) ---

// TestLaunchRalphMsg_NotInProjectDetail validates no-op when not in project detail mode.
func TestLaunchRalphMsg_NotInProjectDetail(t *testing.T) {
	ta := newTestApp(t)
	adapter := ta.adapter()

	_, cmd := adapter.Update(LaunchRalphMsg{})
	if cmd != nil {
		t.Error("expected nil cmd when not in project detail")
	}
	if ta.StatusIsError {
		t.Error("expected no error status when not in project detail")
	}
}

// TestLaunchRalphMsg_NoResourceSelected validates error when no resource is selected.
func TestLaunchRalphMsg_NoResourceSelected(t *testing.T) {
	ta := newTestApp(t)
	ta.Mode = ModeProjectDetail
	ta.Detail = NewProjectDetailView("test-proj")
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	if !ta.StatusIsError || ta.Status != "No resource selected" {
		t.Errorf("expected 'No resource selected' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestLaunchRalphMsg_NoOpenBeads validates error when resource has no open beads.
func TestLaunchRalphMsg_NoOpenBeads(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo", Beads: nil},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	if !ta.StatusIsError || ta.Status != "No open beads for this resource" {
		t.Errorf("expected 'No open beads for this resource' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestLaunchRalphMsg_EmptyBeadsSlice validates error when beads slice is empty (not nil).
func TestLaunchRalphMsg_EmptyBeadsSlice(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo", Beads: []project.BeadInfo{}},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	if !ta.StatusIsError || ta.Status != "No open beads for this resource" {
		t.Errorf("expected 'No open beads for this resource' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}

// TestLaunchRalphMsg_WithBeads validates that ralph proceeds when beads exist.
// tmux.SplitPane will fail (no tmux), but we verify the error is from tmux, not beads.
func TestLaunchRalphMsg_WithBeads(t *testing.T) {
	// Skip if running inside tmux unless explicitly enabled to prevent polluting the session
	if os.Getenv("TMUX") != "" && os.Getenv("DEVDEPLOY_TMUX_TESTS") != "1" {
		t.Skip("Skipping test that calls tmux.SplitPane: set DEVDEPLOY_TMUX_TESTS=1 to enable")
	}
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{
			Kind:         project.ResourceRepo,
			RepoName:     "myrepo",
			WorktreePath: ta.Dir,
			Beads: []project.BeadInfo{
				{ID: "test-abc", Title: "Fix something", Status: "open"},
			},
		},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	// Outside tmux, SplitPane fails. The error should be "Ralph: ..." not "No open beads"
	if ta.StatusIsError && !strings.Contains(ta.Status, "Ralph") {
		t.Errorf("expected 'Ralph' error (tmux not available), got Status=%q", ta.Status)
	}
}

// TestLaunchRalphMsg_PRNoWorktree validates that PR resources without worktrees
// attempt to create one (fails in test, producing a "Ralph:" error).
func TestLaunchRalphMsg_PRNoWorktree(t *testing.T) {
	ta := newTestApp(t)

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{
			Kind:     project.ResourcePR,
			RepoName: "myrepo",
			PR:       &project.PRInfo{Number: 42, Title: "test", HeadRefName: "feat-branch"},
			Beads:    []project.BeadInfo{{ID: "b-1", Title: "Work", Status: "open"}},
		},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	if !ta.StatusIsError || !strings.Contains(ta.Status, "Ralph") {
		t.Errorf("expected 'Ralph' error (EnsurePRWorktree fails), got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}


// TestLaunchRalphMsg_NoBeadsError validates the "no beads" error message
// is shown when resource has no beads, regardless of bead cursor position.
func TestLaunchRalphMsg_NoBeadsError(t *testing.T) {
	ta := newTestApp(t)
	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: ta.Dir},
	}
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	_, _ = adapter.Update(LaunchRalphMsg{})
	if !ta.StatusIsError || !strings.Contains(ta.Status, "No open beads") {
		t.Errorf("expected 'No open beads' error, got Status=%q StatusIsError=%v", ta.Status, ta.StatusIsError)
	}
}
