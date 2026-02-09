package ui

import (
	"strings"
	"testing"

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

	if v.Selected != 0 {
		t.Fatalf("expected initial Selected=0, got %d", v.Selected)
	}

	// j moves down
	v.Update(keyMsg("j"))
	if v.Selected != 1 {
		t.Errorf("after j: expected Selected=1, got %d", v.Selected)
	}
	v.Update(keyMsg("j"))
	if v.Selected != 2 {
		t.Errorf("after j j: expected Selected=2, got %d", v.Selected)
	}

	// k moves up
	v.Update(keyMsg("k"))
	if v.Selected != 1 {
		t.Errorf("after k: expected Selected=1, got %d", v.Selected)
	}

	// k at 0 stays at 0
	v.Update(keyMsg("k"))
	if v.Selected != 0 {
		t.Fatalf("expected Selected=0 after second k, got %d", v.Selected)
	}
	v.Update(keyMsg("k"))
	if v.Selected != 0 {
		t.Errorf("k at top: expected Selected=0, got %d", v.Selected)
	}

	// j at bottom stays at bottom
	v.Selected = len(v.Resources) - 1
	v.Update(keyMsg("j"))
	if v.Selected != len(v.Resources)-1 {
		t.Errorf("j at bottom: expected Selected=%d, got %d", len(v.Resources)-1, v.Selected)
	}
}

func TestProjectDetailView_GAndShiftG(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()
	v.Selected = 2

	// G jumps to last
	v.Update(keyMsg("G"))
	if v.Selected != len(v.Resources)-1 {
		t.Errorf("after G: expected Selected=%d, got %d", len(v.Resources)-1, v.Selected)
	}

	// g jumps to first
	v.Update(keyMsg("g"))
	if v.Selected != 0 {
		t.Errorf("after g: expected Selected=0, got %d", v.Selected)
	}
}

func TestProjectDetailView_NavigationWithEmptyResources(t *testing.T) {
	v := NewProjectDetailView("empty-proj")
	// No resources
	v.Update(keyMsg("j"))
	if v.Selected != 0 {
		t.Errorf("j with no resources: expected Selected=0, got %d", v.Selected)
	}
	v.Update(keyMsg("k"))
	if v.Selected != 0 {
		t.Errorf("k with no resources: expected Selected=0, got %d", v.Selected)
	}
	v.Update(keyMsg("G"))
	if v.Selected != 0 {
		t.Errorf("G with no resources: expected Selected=0, got %d", v.Selected)
	}
}

func TestProjectDetailView_SelectedResource(t *testing.T) {
	v := NewProjectDetailView("my-project")
	v.Resources = testResources()

	r := v.SelectedResource()
	if r == nil || r.Kind != project.ResourceRepo || r.RepoName != "devdeploy" {
		t.Errorf("expected first resource (devdeploy repo), got %+v", r)
	}

	v.Selected = 1
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
	v.Selected = 0

	output := v.View()
	// Selected repo should have ▸ cursor
	if !strings.Contains(output, "▸") {
		t.Error("expected ▸ cursor in view output")
	}

	// Move to PR
	v.Selected = 1
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
