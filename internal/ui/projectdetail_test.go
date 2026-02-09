package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"devdeploy/internal/project"
)

// testResources returns a mixed list of repo and PR resources for testing.
func testResources() []project.Resource {
	return []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
		{Kind: project.ResourcePR, RepoName: "devdeploy", PR: &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"}},
		{Kind: project.ResourcePR, RepoName: "devdeploy", PR: &project.PRInfo{Number: 41, Title: "Fix bug", State: "OPEN"}},
		{Kind: project.ResourceRepo, RepoName: "grafana"},
	}
}

func TestProjectDetailView_JKNavigation(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()

	if v.Selected() != 0 {
		t.Fatalf("expected initial Selected=0, got %d", v.Selected())
	}

	// j moves down
	v.Update(keyMsg("j"))
	if v.Selected() != 1 {
		t.Errorf("after j: expected Selected=1, got %d", v.Selected())
	}
	v.Update(keyMsg("j"))
	if v.Selected() != 2 {
		t.Errorf("after j j: expected Selected=2, got %d", v.Selected())
	}

	// k moves up
	v.Update(keyMsg("k"))
	if v.Selected() != 1 {
		t.Errorf("after k: expected Selected=1, got %d", v.Selected())
	}

	// k at 0 stays at 0
	v.Update(keyMsg("k"))
	if v.Selected() != 0 {
		t.Fatalf("expected Selected=0 after second k, got %d", v.Selected())
	}
	v.Update(keyMsg("k"))
	if v.Selected() != 0 {
		t.Errorf("k at top: expected Selected=0, got %d", v.Selected())
	}

	// j at bottom stays at bottom
	v.setSelected(len(v.Resources) - 1)
	v.Update(keyMsg("j"))
	if v.Selected() != len(v.Resources)-1 {
		t.Errorf("j at bottom: expected Selected=%d, got %d", len(v.Resources)-1, v.Selected())
	}
}

func TestProjectDetailView_GAndShiftG(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()
	v.setSelected(2)

	// G jumps to last
	v.Update(keyMsg("G"))
	if v.Selected() != len(v.Resources)-1 {
		t.Errorf("after G: expected Selected=%d, got %d", len(v.Resources)-1, v.Selected())
	}

	// g jumps to first
	v.Update(keyMsg("g"))
	if v.Selected() != 0 {
		t.Errorf("after g: expected Selected=0, got %d", v.Selected())
	}
}

func TestProjectDetailView_NavigationWithEmptyResources(t *testing.T) {
	v := NewProjectDetailView("empty-proj")
	// No resources
	v.Update(keyMsg("j"))
	if v.Selected() != 0 {
		t.Errorf("j with no resources: expected Selected=0, got %d", v.Selected())
	}
	v.Update(keyMsg("k"))
	if v.Selected() != 0 {
		t.Errorf("k with no resources: expected Selected=0, got %d", v.Selected())
	}
	v.Update(keyMsg("G"))
	if v.Selected() != 0 {
		t.Errorf("G with no resources: expected Selected=0, got %d", v.Selected())
	}
}

func TestProjectDetailView_SelectedResource(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()

	r := v.SelectedResource()
	if r == nil || r.Kind != project.ResourceRepo || r.RepoName != "devdeploy" {
		t.Errorf("expected first resource (devdeploy repo), got %+v", r)
	}

	v.setSelected(1)
	r = v.SelectedResource()
	if r == nil || r.Kind != project.ResourcePR || r.PR.Number != 42 {
		t.Errorf("expected PR #42, got %+v", r)
	}

	// Out-of-range returns nil
	v2 := NewProjectDetailView("empty")
	if v2.SelectedResource() != nil {
		t.Error("expected nil for empty resources")
	}
}

func TestProjectDetailView_ViewSelectionCursor(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()
	v.setSelected(0)

	output := v.View()
	// Selected repo should have ▸ cursor
	if !strings.Contains(output, "▸") {
		t.Error("expected ▸ cursor in view output")
	}

	// Move to PR
	v.setSelected(1)
	output = v.View()
	if !strings.Contains(output, "▸") {
		t.Error("expected ▸ cursor on PR row")
	}
	if !strings.Contains(output, "#42") {
		t.Error("expected #42 in view output")
	}
}

func TestProjectDetailView_ViewStatusIndicators(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{
			Kind:         project.ResourceRepo,
			RepoName:     "devdeploy",
			WorktreePath: "/tmp/devdeploy",
			Panes: []project.PaneInfo{
				{ID: "%1", IsAgent: false},
				{ID: "%2", IsAgent: false},
			},
		},
		{
			Kind:     project.ResourcePR,
			RepoName: "devdeploy",
			PR:       &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"},
			Panes: []project.PaneInfo{
				{ID: "%3", IsAgent: true},
			},
		},
		{Kind: project.ResourceRepo, RepoName: "grafana"}, // no worktree, no panes
	}

	output := v.View()

	// Repo with 2 shells should show "● 2 shells"
	if !strings.Contains(output, "2 shells") {
		t.Errorf("expected '2 shells' indicator, got:\n%s", output)
	}

	// PR with 1 agent should show "● 1 agent"
	if !strings.Contains(output, "1 agent") {
		t.Errorf("expected '1 agent' indicator, got:\n%s", output)
	}

	// grafana has no worktree and no panes — should NOT have ●
	// Split by lines and check the grafana line
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "grafana") && strings.Contains(line, "●") {
			t.Errorf("grafana should not have ● indicator, got: %s", line)
		}
	}
}

func TestResourceStatus(t *testing.T) {
	tests := []struct {
		name     string
		resource project.Resource
		want     string
	}{
		{
			name:     "no worktree no panes",
			resource: project.Resource{Kind: project.ResourceRepo, RepoName: "foo"},
			want:     "",
		},
		{
			name:     "worktree no panes",
			resource: project.Resource{Kind: project.ResourceRepo, RepoName: "foo", WorktreePath: "/tmp/foo"},
			want:     "●",
		},
		{
			name: "1 shell",
			resource: project.Resource{
				Kind: project.ResourceRepo, RepoName: "foo",
				Panes: []project.PaneInfo{{ID: "%1", IsAgent: false}},
			},
			want: "● 1 shell",
		},
		{
			name: "2 shells 1 agent",
			resource: project.Resource{
				Kind: project.ResourceRepo, RepoName: "foo",
				Panes: []project.PaneInfo{
					{ID: "%1", IsAgent: false},
					{ID: "%2", IsAgent: false},
					{ID: "%3", IsAgent: true},
				},
			},
			want: "● 2 shells 1 agent",
		},
		{
			name: "2 agents",
			resource: project.Resource{
				Kind: project.ResourceRepo, RepoName: "foo",
				Panes: []project.PaneInfo{
					{ID: "%1", IsAgent: true},
					{ID: "%2", IsAgent: true},
				},
			},
			want: "● 2 agents",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceStatus(tt.resource)
			if got != tt.want {
				t.Errorf("resourceStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectDetailView_NoReposMessage(t *testing.T) {
	v := NewProjectDetailView("empty")
	output := v.View()
	if !strings.Contains(output, "(no repos added)") {
		t.Errorf("expected '(no repos added)' for empty resources, got:\n%s", output)
	}
}

func TestProjectDetailView_BeadsRenderedUnderRepo(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{
			Kind:         project.ResourceRepo,
			RepoName:     "devdeploy",
			WorktreePath: "/tmp/devdeploy",
			Beads: []project.BeadInfo{
				{ID: "devdeploy-abc", Title: "Fix the thing", Status: "open"},
				{ID: "devdeploy-def", Title: "Add feature X", Status: "in_progress"},
			},
		},
	}

	output := v.View()

	if !strings.Contains(output, "devdeploy-abc") {
		t.Errorf("expected bead ID 'devdeploy-abc' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "Fix the thing") {
		t.Errorf("expected bead title 'Fix the thing' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "devdeploy-def") {
		t.Errorf("expected bead ID 'devdeploy-def' in output, got:\n%s", output)
	}
	// in_progress status should show as a tag
	if !strings.Contains(output, "[in_progress]") {
		t.Errorf("expected '[in_progress]' status tag in output, got:\n%s", output)
	}
	// open status should NOT show as a tag (it's the default)
	if strings.Contains(output, "[open]") {
		t.Errorf("expected no '[open]' tag (default status), got:\n%s", output)
	}
}

func TestProjectDetailView_BeadsRenderedUnderPR(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
		{
			Kind:     project.ResourcePR,
			RepoName: "devdeploy",
			PR:       &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"},
			Beads: []project.BeadInfo{
				{ID: "devdeploy-ghi", Title: "Review feedback", Status: "open"},
			},
		},
	}

	output := v.View()

	if !strings.Contains(output, "devdeploy-ghi") {
		t.Errorf("expected bead 'devdeploy-ghi' under PR, got:\n%s", output)
	}
	if !strings.Contains(output, "Review feedback") {
		t.Errorf("expected bead title 'Review feedback' under PR, got:\n%s", output)
	}
}

func TestProjectDetailView_NoBeadsNoPlaceholder(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
	}

	output := v.View()

	// Should not contain any bead-related placeholder
	if strings.Contains(output, "no beads") {
		t.Errorf("expected no 'no beads' placeholder, got:\n%s", output)
	}
}

func TestProjectDetailView_BeadTitleTruncation(t *testing.T) {
	v := NewProjectDetailView("my-project")
	longTitle := "This is a very long bead title that should be truncated because it exceeds the maximum display width"
	v.Resources = []project.Resource{
		{
			Kind:         project.ResourceRepo,
			RepoName:     "devdeploy",
			WorktreePath: "/tmp/devdeploy",
			Beads: []project.BeadInfo{
				{ID: "devdeploy-xyz", Title: longTitle, Status: "open"},
			},
		},
	}

	output := v.View()

	// The full long title should not appear — it should be truncated
	if strings.Contains(output, longTitle) {
		t.Errorf("expected long bead title to be truncated, got:\n%s", output)
	}
	if !strings.Contains(output, "...") {
		t.Errorf("expected '...' truncation marker, got:\n%s", output)
	}
}

// testResourcesWithBeads returns resources with beads for two-level cursor testing.
func testResourcesWithBeads() []project.Resource {
	return []project.Resource{
		{
			Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy",
			Beads: []project.BeadInfo{
				{ID: "devdeploy-abc", Title: "Fix the thing", Status: "open"},
				{ID: "devdeploy-def", Title: "Add feature X", Status: "in_progress"},
			},
		},
		{
			Kind: project.ResourcePR, RepoName: "devdeploy",
			PR:    &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"},
			Beads: []project.BeadInfo{{ID: "devdeploy-ghi", Title: "Review feedback", Status: "open"}},
		},
		{Kind: project.ResourceRepo, RepoName: "grafana"}, // no beads
	}
}

func TestProjectDetailView_BeadNavigation_JK(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = testResourcesWithBeads()
	v.buildItems() // Build items after setting resources

	// Start on resource 0 header.
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != -1 {
		t.Fatalf("initial: expected (0,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j → resource 0, bead 0
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != 0 {
		t.Errorf("j from header: expected (0,0), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j → resource 0, bead 1
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != 1 {
		t.Errorf("j from bead 0: expected (0,1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j → resource 1 header (last bead of resource 0 → next resource)
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != -1 {
		t.Errorf("j from last bead: expected (1,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j → resource 1, bead 0
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != 0 {
		t.Errorf("j from r1 header: expected (1,0), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j → resource 2 header (no beads)
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 2 || v.SelectedBeadIdx() != -1 {
		t.Errorf("j from r1 bead: expected (2,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// j at bottom stays at bottom
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 2 || v.SelectedBeadIdx() != -1 {
		t.Errorf("j at bottom: expected (2,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// Now go back up with k
	// k → resource 1, bead 0 (last bead of previous resource)
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != 0 {
		t.Errorf("k from r2 header: expected (1,0), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// k → resource 1 header
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != -1 {
		t.Errorf("k from r1 bead 0: expected (1,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// k → resource 0, bead 1 (last bead of r0)
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != 1 {
		t.Errorf("k from r1 header: expected (0,1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// k → resource 0, bead 0
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != 0 {
		t.Errorf("k from bead 1: expected (0,0), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// k → resource 0 header
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != -1 {
		t.Errorf("k from bead 0: expected (0,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// k at top stays at top
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != -1 {
		t.Errorf("k at top: expected (0,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
}

func TestProjectDetailView_BeadNavigation_GAndShiftG(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = testResourcesWithBeads()
	v.buildItems() // Build items after setting resources

	// Move to middle (resource 1, bead 0)
	// Find the item index for resource 1, bead 0
	var targetIdx int
	for i, item := range v.items {
		if item.resourceIdx == 1 && item.itemType == itemTypeBead && item.beadIdx == 0 {
			targetIdx = i
			break
		}
	}
	v.setSelected(targetIdx)

	// g → first resource header
	v.Update(keyMsg("g"))
	if v.SelectedResourceIdx() != 0 || v.SelectedBeadIdx() != -1 {
		t.Errorf("g: expected (0,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}

	// G → last resource header (grafana has no beads)
	v.Update(keyMsg("G"))
	if v.SelectedResourceIdx() != 2 || v.SelectedBeadIdx() != -1 {
		t.Errorf("G: expected (2,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
}

func TestProjectDetailView_BeadNavigation_GJumpsToLastBead(t *testing.T) {
	// Test G when last resource has beads.
	v := NewProjectDetailView("proj")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "foo"},
		{
			Kind: project.ResourceRepo, RepoName: "bar",
			Beads: []project.BeadInfo{
				{ID: "b-1", Title: "First"},
				{ID: "b-2", Title: "Second"},
			},
		},
	}
	v.buildItems() // Build items after setting resources

	v.Update(keyMsg("G"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != 1 {
		t.Errorf("G with beads on last: expected (1,1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
}

func TestProjectDetailView_SelectedBead(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = testResourcesWithBeads()
	v.buildItems() // Build items after setting resources

	// On resource header — no bead selected.
	if v.SelectedBead() != nil {
		t.Error("expected nil bead on resource header")
	}

	// Move to bead 0.
	v.Update(keyMsg("j"))
	bd := v.SelectedBead()
	if bd == nil || bd.ID != "devdeploy-abc" {
		t.Errorf("expected devdeploy-abc, got %+v", bd)
	}

	// Move to bead 1.
	v.Update(keyMsg("j"))
	bd = v.SelectedBead()
	if bd == nil || bd.ID != "devdeploy-def" {
		t.Errorf("expected devdeploy-def, got %+v", bd)
	}

	// Move to resource 1 header — no bead.
	v.Update(keyMsg("j"))
	if v.SelectedBead() != nil {
		t.Error("expected nil bead on PR header")
	}

	// Move to resource 1, bead 0.
	v.Update(keyMsg("j"))
	bd = v.SelectedBead()
	if bd == nil || bd.ID != "devdeploy-ghi" {
		t.Errorf("expected devdeploy-ghi, got %+v", bd)
	}

	// Empty view.
	v2 := NewProjectDetailView("empty")
	if v2.SelectedBead() != nil {
		t.Error("expected nil bead for empty view")
	}
}

func TestProjectDetailView_SelectedBead_IssueType(t *testing.T) {
	// Test that IssueType is accessible via SelectedBead() for epic detection
	v := NewProjectDetailView("proj")
	v.Resources = []project.Resource{
		{
			Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy",
			Beads: []project.BeadInfo{
				{ID: "epic-1", Title: "Epic One", Status: "open", IssueType: "epic"},
				{ID: "task-1", Title: "Task One", Status: "open", IssueType: "task"},
			},
		},
	}
	v.buildItems()

	// Move to epic bead.
	v.Update(keyMsg("j"))
	bd := v.SelectedBead()
	if bd == nil || bd.ID != "epic-1" {
		t.Fatalf("expected epic-1, got %+v", bd)
	}
	if bd.IssueType != "epic" {
		t.Errorf("expected IssueType 'epic', got %q", bd.IssueType)
	}

	// Move to task bead.
	v.Update(keyMsg("j"))
	bd = v.SelectedBead()
	if bd == nil || bd.ID != "task-1" {
		t.Fatalf("expected task-1, got %+v", bd)
	}
	if bd.IssueType != "task" {
		t.Errorf("expected IssueType 'task', got %q", bd.IssueType)
	}
}

func TestProjectDetailView_BeadSelectionHighlight(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = testResourcesWithBeads()

	// Move to bead 0.
	v.Update(keyMsg("j"))
	output := v.View()

	// Selected bead should have ▸ cursor.
	if !strings.Contains(output, "▸") {
		t.Error("expected ▸ cursor on selected bead")
	}
	// The bead ID should appear.
	if !strings.Contains(output, "devdeploy-abc") {
		t.Error("expected devdeploy-abc in output")
	}
}

func TestProjectDetailView_NoBeadsResourceNavUnchanged(t *testing.T) {
	// When no resources have beads, behavior should be like old resource-only navigation.
	v := NewProjectDetailView("proj")
	v.Resources = testResources() // no beads
	v.buildItems() // Build items after setting resources

	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != -1 {
		t.Errorf("j: expected (1,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
	v.Update(keyMsg("j"))
	if v.SelectedResourceIdx() != 2 || v.SelectedBeadIdx() != -1 {
		t.Errorf("j: expected (2,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
	v.Update(keyMsg("k"))
	if v.SelectedResourceIdx() != 1 || v.SelectedBeadIdx() != -1 {
		t.Errorf("k: expected (1,-1), got (%d,%d)", v.SelectedResourceIdx(), v.SelectedBeadIdx())
	}
}

func TestProjectDetailView_ChildBeadsIndented(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{
			Kind:         project.ResourceRepo,
			RepoName:     "devdeploy",
			WorktreePath: "/tmp/devdeploy",
			Beads: []project.BeadInfo{
				{ID: "epic-1", Title: "Epic One", Status: "open", IssueType: "epic"},
				{ID: "child-1", Title: "Child One", Status: "open", IsChild: true},
				{ID: "standalone", Title: "Standalone task", Status: "open"},
			},
		},
	}

	output := v.View()

	// All bead IDs should be present.
	if !strings.Contains(output, "epic-1") {
		t.Errorf("expected epic-1 in output, got:\n%s", output)
	}
	if !strings.Contains(output, "child-1") {
		t.Errorf("expected child-1 in output, got:\n%s", output)
	}
	if !strings.Contains(output, "standalone") {
		t.Errorf("expected standalone in output, got:\n%s", output)
	}

	// Check that child bead line has more leading spaces than the epic line.
	lines := strings.Split(output, "\n")
	var epicLine, childLine string
	for _, line := range lines {
		if strings.Contains(line, "epic-1") {
			epicLine = line
		}
		if strings.Contains(line, "child-1") {
			childLine = line
		}
	}
	if epicLine == "" || childLine == "" {
		t.Fatalf("could not find epic/child lines in output:\n%s", output)
	}
	epicIndent := len(epicLine) - len(strings.TrimLeft(epicLine, " "))
	childIndent := len(childLine) - len(strings.TrimLeft(childLine, " "))
	if childIndent <= epicIndent {
		t.Errorf("child bead should have more indent (%d) than epic (%d)\nepic:  %q\nchild: %q", childIndent, epicIndent, epicLine, childLine)
	}
}

// --- Scroll / viewport tests ---

// TestProjectDetailView_CursorRow is skipped - cursorRow() method doesn't exist in current implementation
// func TestProjectDetailView_CursorRow(t *testing.T) {
// 	// This test was for a cursorRow() method that no longer exists
// 	t.Skip("cursorRow() method removed")
// }

func TestProjectDetailView_NoScrollWhenFits(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = testResources() // 4 resources, no beads
	v.SetSize(80, 30)             // plenty of room

	output := v.View()

	// Should NOT have scroll indicators.
	if strings.Contains(output, "↑") || strings.Contains(output, "↓") {
		t.Errorf("expected no scroll indicators when content fits, got:\n%s", output)
	}
}

func TestProjectDetailView_ScrollsDownWhenCursorMovesBelow(t *testing.T) {
	v := NewProjectDetailView("proj")
	// Create many resources to exceed viewport.
	var resources []project.Resource
	for i := 0; i < 20; i++ {
		resources = append(resources, project.Resource{
			Kind:     project.ResourceRepo,
			RepoName: fmt.Sprintf("repo-%02d", i),
		})
	}
	v.Resources = resources
	v.SetSize(80, 12) // Small terminal: viewHeight = 12 - 4 = 8 lines

	// Navigate to the bottom.
	for i := 0; i < 19; i++ {
		v.Update(keyMsg("j"))
	}

	output := v.View()

	// Last resource should be visible.
	if !strings.Contains(output, "repo-19") {
		t.Errorf("expected repo-19 visible after scrolling down, got:\n%s", output)
	}
	// Verify scrolling occurred - earlier repos should not be visible
	if strings.Contains(output, "repo-00") {
		t.Errorf("expected repo-00 not visible after scrolling down (viewport should have scrolled), got:\n%s", output)
	}
}

func TestProjectDetailView_ScrollsUpWhenCursorMovesAbove(t *testing.T) {
	v := NewProjectDetailView("proj")
	var resources []project.Resource
	for i := 0; i < 20; i++ {
		resources = append(resources, project.Resource{
			Kind:     project.ResourceRepo,
			RepoName: fmt.Sprintf("repo-%02d", i),
		})
	}
	v.Resources = resources
	v.SetSize(80, 12) // viewHeight = 8

	// Navigate to bottom first.
	for i := 0; i < 19; i++ {
		v.Update(keyMsg("j"))
	}

	// Now navigate back to top.
	for i := 0; i < 19; i++ {
		v.Update(keyMsg("k"))
	}

	output := v.View()

	// First resource should be visible.
	if !strings.Contains(output, "repo-00") {
		t.Errorf("expected repo-00 visible after scrolling back up, got:\n%s", output)
	}
	// Verify scrolling occurred - later repos should not be visible
	if strings.Contains(output, "repo-19") {
		t.Errorf("expected repo-19 not visible after scrolling back up (viewport should have scrolled), got:\n%s", output)
	}
}

func TestProjectDetailView_GAndShiftG_Scroll(t *testing.T) {
	v := NewProjectDetailView("proj")
	var resources []project.Resource
	for i := 0; i < 20; i++ {
		resources = append(resources, project.Resource{
			Kind:     project.ResourceRepo,
			RepoName: fmt.Sprintf("repo-%02d", i),
		})
	}
	v.Resources = resources
	v.SetSize(80, 12)

	// G jumps to bottom.
	v.Update(keyMsg("G"))
	output := v.View()
	if !strings.Contains(output, "repo-19") {
		t.Errorf("G: expected repo-19 visible, got:\n%s", output)
	}

	// g jumps to top.
	v.Update(keyMsg("g"))
	output = v.View()
	if !strings.Contains(output, "repo-00") {
		t.Errorf("g: expected repo-00 visible, got:\n%s", output)
	}
}

func TestProjectDetailView_ScrollWithBeads(t *testing.T) {
	v := NewProjectDetailView("proj")
	v.Resources = []project.Resource{
		{
			Kind: project.ResourceRepo, RepoName: "repo-a", WorktreePath: "/tmp/a",
			Beads: []project.BeadInfo{
				{ID: "a-1", Title: "Bead 1"},
				{ID: "a-2", Title: "Bead 2"},
				{ID: "a-3", Title: "Bead 3"},
			},
		},
		{
			Kind: project.ResourceRepo, RepoName: "repo-b", WorktreePath: "/tmp/b",
			Beads: []project.BeadInfo{
				{ID: "b-1", Title: "Bead 4"},
				{ID: "b-2", Title: "Bead 5"},
				{ID: "b-3", Title: "Bead 6"},
			},
		},
		{
			Kind: project.ResourceRepo, RepoName: "repo-c", WorktreePath: "/tmp/c",
			Beads: []project.BeadInfo{
				{ID: "c-1", Title: "Bead 7"},
			},
		},
	}
	v.SetSize(80, 10) // viewHeight = 6

	// Navigate down through beads until repo-c's bead is selected.
	// Layout: header(3 lines) + repo-a(1) + 3 beads + repo-b(1) + 3 beads + repo-c(1) + 1 bead = 13 lines
	// Navigate: start at (0,-1), j→(0,0), j→(0,1), j→(0,2), j→(1,-1), j→(1,0), j→(1,1), j→(1,2), j→(2,-1), j→(2,0)
	for i := 0; i < 10; i++ {
		v.Update(keyMsg("j"))
	}

	output := v.View()

	// c-1 bead should be visible.
	if !strings.Contains(output, "c-1") {
		t.Errorf("expected c-1 bead visible after scrolling, got:\n%s", output)
	}
}

func TestProjectDetailView_NoScrollWithoutTermHeight(t *testing.T) {
	v := NewProjectDetailView("proj")
	var resources []project.Resource
	for i := 0; i < 30; i++ {
		resources = append(resources, project.Resource{
			Kind:     project.ResourceRepo,
			RepoName: fmt.Sprintf("repo-%02d", i),
		})
	}
	v.Resources = resources
	// termHeight is 0 (default) — no scrolling.

	output := v.View()

	// All repos should be visible (no clipping).
	if !strings.Contains(output, "repo-00") || !strings.Contains(output, "repo-29") {
		t.Errorf("expected all repos visible without termHeight, got:\n%s", output)
	}
	// No scroll indicators.
	if strings.Contains(output, "↑") || strings.Contains(output, "↓") {
		t.Errorf("expected no scroll indicators without termHeight, got:\n%s", output)
	}
}

func TestProjectDetailView_WindowSizeMsgUpdatesViewport(t *testing.T) {
	v := NewProjectDetailView("proj")
	var resources []project.Resource
	for i := 0; i < 20; i++ {
		resources = append(resources, project.Resource{
			Kind:     project.ResourceRepo,
			RepoName: fmt.Sprintf("repo-%02d", i),
		})
	}
	v.Resources = resources

	// Send WindowSizeMsg.
	v.Update(tea.WindowSizeMsg{Width: 80, Height: 12})

	// Navigate to bottom.
	v.Update(keyMsg("G"))
	output := v.View()

	if !strings.Contains(output, "repo-19") {
		t.Errorf("expected repo-19 visible after WindowSizeMsg + G, got:\n%s", output)
	}
}

func TestProjectDetailView_WiderTerminalShowsMoreContent(t *testing.T) {
	longTitle := "This is a very long PR title that would be truncated on a narrow terminal but should be fully visible on a wide one"
	longBeadTitle := "This is a very long bead title that should expand on wider terminals instead of being cut short"

	makePRView := func(width int) *ProjectDetailView {
		v := NewProjectDetailView("proj")
		v.Resources = []project.Resource{
			{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy",
				Beads: []project.BeadInfo{
					{ID: "devdeploy-xyz", Title: longBeadTitle, Status: "open"},
				},
			},
			{Kind: project.ResourcePR, RepoName: "devdeploy",
				PR: &project.PRInfo{Number: 99, Title: longTitle, State: "OPEN"}},
		}
		if width > 0 {
			v.SetSize(width, 30)
		}
		return v
	}

	// Default (no width set) — uses hard-coded fallbacks.
	defaultView := makePRView(0)
	defaultOutput := defaultView.View()

	// Narrow terminal (80 cols).
	narrowView := makePRView(80)
	narrowOutput := narrowView.View()

	// Wide terminal (200 cols).
	wideView := makePRView(200)
	wideOutput := wideView.View()

	// The wide terminal should show more of the long PR title than narrow.
	// Both should truncate, but wide should have more visible characters.
	narrowPRTruncated := !strings.Contains(narrowOutput, longTitle)
	widePRTruncated := !strings.Contains(wideOutput, longTitle)

	if !narrowPRTruncated {
		t.Error("expected PR title to be truncated on narrow terminal")
	}
	if widePRTruncated {
		t.Errorf("expected full PR title visible on wide terminal (200 cols), got:\n%s", wideOutput)
	}

	// The wide terminal should show more of the bead title.
	narrowBeadTruncated := !strings.Contains(narrowOutput, longBeadTitle)
	wideBeadTruncated := !strings.Contains(wideOutput, longBeadTitle)

	if !narrowBeadTruncated {
		t.Error("expected bead title to be truncated on narrow terminal")
	}
	if wideBeadTruncated {
		t.Errorf("expected full bead title visible on wide terminal (200 cols), got:\n%s", wideOutput)
	}

	// Default (no width) should still truncate like before.
	if strings.Contains(defaultOutput, longTitle) {
		t.Error("expected PR title truncated with default (no width)")
	}
	if strings.Contains(defaultOutput, longBeadTitle) {
		t.Error("expected bead title truncated with default (no width)")
	}
}

// --- Filter mode tests (devdeploy-fyt.3) ---

func TestProjectDetailView_FilterFlow(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
		{Kind: project.ResourceRepo, RepoName: "grafana", WorktreePath: "/tmp/grafana"},
		{Kind: project.ResourcePR, RepoName: "devdeploy",
			PR: &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"}},
	}
	v.buildItems()

	// Initially not filtering
	if v.IsFiltering() {
		t.Fatal("expected not filtering initially")
	}

	// Press / to start filtering
	v.Update(keyMsg("/"))
	if !v.IsFiltering() {
		t.Fatal("expected filtering after pressing /")
	}

	// Type "dev" to filter
	v.Update(keyMsg("d"))
	v.Update(keyMsg("e"))
	v.Update(keyMsg("v"))

	// Still filtering (user is typing)
	if !v.IsFiltering() {
		t.Fatal("expected still filtering while typing")
	}

	// Press enter to apply filter
	v.Update(keyMsg("enter"))

	// After enter, filter should be applied (not actively filtering, but filter is active)
	// The list should be in FilterApplied state, not Filtering state
	if v.IsFiltering() {
		t.Error("expected not actively filtering after enter (filter applied)")
	}

	// Verify filter is applied - only items matching "dev" should be visible
	// Check that SelectedResource returns a filtered item
	selected := v.SelectedResource()
	if selected == nil {
		t.Fatal("expected a selected resource after filtering")
	}
	// Selected resource should match filter (devdeploy repo or PR)
	if selected.RepoName != "devdeploy" {
		t.Errorf("expected selected resource to match filter 'dev', got %q", selected.RepoName)
	}
}

func TestProjectDetailView_CancelFilter(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
		{Kind: project.ResourceRepo, RepoName: "grafana", WorktreePath: "/tmp/grafana"},
	}
	v.buildItems()

	initialSelected := v.Selected()

	// Press / to start filtering
	v.Update(keyMsg("/"))
	if !v.IsFiltering() {
		t.Fatal("expected filtering after pressing /")
	}

	// Type "dev" to filter
	v.Update(keyMsg("d"))
	v.Update(keyMsg("e"))
	v.Update(keyMsg("v"))

	// Press esc to cancel filter
	v.Update(keyMsg("esc"))

	// Filter should be cleared, not actively filtering
	if v.IsFiltering() {
		t.Error("expected not filtering after esc")
	}

	// Should still be in detail view (not navigated away)
	// Verify by checking that we can still navigate
	v.Update(keyMsg("j"))
	if v.Selected() == initialSelected {
		t.Error("expected navigation to work after canceling filter")
	}
}

func TestProjectDetailView_NavigationAfterFilter(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "devdeploy", WorktreePath: "/tmp/devdeploy"},
		{Kind: project.ResourceRepo, RepoName: "grafana", WorktreePath: "/tmp/grafana"},
		{Kind: project.ResourcePR, RepoName: "devdeploy",
			PR: &project.PRInfo{Number: 42, Title: "Add dark mode", State: "OPEN"}},
	}
	v.buildItems()

	// Apply filter: / -> type "dev" -> enter
	v.Update(keyMsg("/"))
	v.Update(keyMsg("d"))
	v.Update(keyMsg("e"))
	v.Update(keyMsg("v"))
	v.Update(keyMsg("enter"))

	// Verify filter is applied (not actively filtering)
	if v.IsFiltering() {
		t.Error("expected not actively filtering after applying filter")
	}

	// Get initial selection after filter
	initialIdx := v.Selected()
	initialResource := v.SelectedResource()

	// j navigation should work (doesn't error, moves selection)
	v.Update(keyMsg("j"))
	if v.Selected() == initialIdx {
		t.Error("expected j navigation to work after filter")
	}
	newResource := v.SelectedResource()
	if newResource == nil {
		t.Fatal("expected a selected resource after j navigation")
	}

	// k navigation should work (returns to previous position)
	v.Update(keyMsg("k"))
	if v.Selected() != initialIdx {
		t.Errorf("expected k navigation to return to initial position, got %d (was %d)", v.Selected(), initialIdx)
	}
	if v.SelectedResource() != initialResource {
		t.Error("expected k navigation to return to initial resource")
	}
}

func TestProjectDetailView_SPCCommandsAfterFilter(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
		{Kind: project.ResourceRepo, RepoName: "otherrepo", WorktreePath: "/tmp/otherrepo"},
	}
	detail.buildItems()
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// Apply filter: / -> type "my" -> enter
	_, _ = adapter.Update(keyMsg("/"))
	_, _ = adapter.Update(keyMsg("m"))
	_, _ = adapter.Update(keyMsg("y"))
	_, _ = adapter.Update(keyMsg("enter"))

	// Verify filter is applied (not actively filtering)
	if ta.Detail.IsFiltering() {
		t.Error("expected not actively filtering after applying filter")
	}

	// SPC commands should work normally
	// Test SPC p (should show project hints)
	_, cmd := adapter.Update(keyMsg(" "))
	if cmd != nil {
		adapter.Update(cmd())
	}
	if !ta.KeyHandler.LeaderWaiting {
		t.Error("expected leader waiting after SPC")
	}

	// Complete SPC p a (add repo) - should work even with filter applied
	_, cmd = adapter.Update(keyMsg("p"))
	if cmd != nil {
		adapter.Update(cmd())
	}
	_, cmd = adapter.Update(keyMsg("a"))
	if cmd != nil {
		adapter.Update(cmd())
	}
	// Should show add repo modal or status (depending on workspace repos)
	// The key is that it doesn't error due to filter state
}

func TestProjectDetailView_EnterAfterFilter(t *testing.T) {
	ta := newTestApp(t)
	_ = ta.ProjectManager.CreateProject("test-proj")

	detail := NewProjectDetailView("test-proj")
	detail.Resources = []project.Resource{
		{Kind: project.ResourceRepo, RepoName: "myrepo", WorktreePath: "/tmp/myrepo"},
		{Kind: project.ResourceRepo, RepoName: "otherrepo", WorktreePath: "/tmp/otherrepo"},
	}
	detail.buildItems()
	detail.setSelected(0)

	ta.Mode = ModeProjectDetail
	ta.Detail = detail
	adapter := ta.adapter()

	// Apply filter: / -> type "my" -> enter
	_, _ = adapter.Update(keyMsg("/"))
	_, _ = adapter.Update(keyMsg("m"))
	_, _ = adapter.Update(keyMsg("y"))
	_, _ = adapter.Update(keyMsg("enter"))

	// Verify filter is applied
	if ta.Detail.IsFiltering() {
		t.Error("expected not actively filtering after applying filter")
	}

	// Verify selected resource matches filter
	selected := ta.Detail.SelectedResource()
	if selected == nil || selected.RepoName != "myrepo" {
		t.Fatalf("expected selected resource to be 'myrepo' after filter, got %+v", selected)
	}

	// Press enter on selected item - should trigger OpenShellMsg
	_, cmd := adapter.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter after filter")
	}
	msg := cmd()
	if _, ok := msg.(OpenShellMsg); !ok {
		t.Errorf("expected OpenShellMsg from Enter after filter, got %T", msg)
	}
}

// TestProjectDetailView_MaxContentLen is skipped - maxContentLen() method doesn't exist in current implementation
// func TestProjectDetailView_MaxContentLen(t *testing.T) {
// 	// This test was for a maxContentLen() method that no longer exists
// 	t.Skip("maxContentLen() method removed")
// }
