package ui

import (
	"strings"
	"testing"
)

func testProjects() []ProjectSummary {
	return []ProjectSummary{
		{Name: "alpha", RepoCount: 2, PRCount: 3},
		{Name: "beta", RepoCount: 1, PRCount: 0},
		{Name: "gamma", RepoCount: 0, PRCount: 1},
	}
}

func TestDashboardView_JKNavigation(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()

	if d.Selected != 0 {
		t.Fatalf("expected initial Selected=0, got %d", d.Selected)
	}

	// j moves down.
	d.Update(keyMsg("j"))
	if d.Selected != 1 {
		t.Errorf("after j: expected Selected=1, got %d", d.Selected)
	}
	d.Update(keyMsg("j"))
	if d.Selected != 2 {
		t.Errorf("after j j: expected Selected=2, got %d", d.Selected)
	}

	// j at bottom stays at bottom.
	d.Update(keyMsg("j"))
	if d.Selected != 2 {
		t.Errorf("j at bottom: expected Selected=2, got %d", d.Selected)
	}

	// k moves up.
	d.Update(keyMsg("k"))
	if d.Selected != 1 {
		t.Errorf("after k: expected Selected=1, got %d", d.Selected)
	}

	// k to top, then again stays at 0.
	d.Update(keyMsg("k"))
	if d.Selected != 0 {
		t.Fatalf("expected Selected=0 after second k, got %d", d.Selected)
	}
	d.Update(keyMsg("k"))
	if d.Selected != 0 {
		t.Errorf("k at top: expected Selected=0, got %d", d.Selected)
	}
}

func TestDashboardView_GAndShiftG(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()

	// G jumps to last.
	d.Update(keyMsg("G"))
	if d.Selected != 2 {
		t.Errorf("after G: expected Selected=2, got %d", d.Selected)
	}

	// g jumps to first.
	d.Update(keyMsg("g"))
	if d.Selected != 0 {
		t.Errorf("after g: expected Selected=0, got %d", d.Selected)
	}

	// G when already at last is a no-op.
	d.Selected = 2
	d.Update(keyMsg("G"))
	if d.Selected != 2 {
		t.Errorf("G at bottom: expected Selected=2, got %d", d.Selected)
	}

	// g when already at first is a no-op.
	d.Selected = 0
	d.Update(keyMsg("g"))
	if d.Selected != 0 {
		t.Errorf("g at top: expected Selected=0, got %d", d.Selected)
	}
}

func TestDashboardView_NavigationWithEmptyProjects(t *testing.T) {
	d := NewDashboardView()
	// No projects.

	d.Update(keyMsg("j"))
	if d.Selected != 0 {
		t.Errorf("j with no projects: expected Selected=0, got %d", d.Selected)
	}
	d.Update(keyMsg("k"))
	if d.Selected != 0 {
		t.Errorf("k with no projects: expected Selected=0, got %d", d.Selected)
	}
	d.Update(keyMsg("G"))
	if d.Selected != 0 {
		t.Errorf("G with no projects: expected Selected=0, got %d", d.Selected)
	}
	d.Update(keyMsg("g"))
	if d.Selected != 0 {
		t.Errorf("g with no projects: expected Selected=0, got %d", d.Selected)
	}
}

func TestDashboardView_DownArrowNavigation(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()

	d.Update(keyMsg("down"))
	if d.Selected != 1 {
		t.Errorf("after down: expected Selected=1, got %d", d.Selected)
	}
	d.Update(keyMsg("up"))
	if d.Selected != 0 {
		t.Errorf("after up: expected Selected=0, got %d", d.Selected)
	}
}

func TestDashboardView_ViewRendersProjectList(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()
	d.Selected = 0

	output := d.View()

	// Should contain the title.
	if !strings.Contains(output, "Projects") {
		t.Error("expected 'Projects' title in view output")
	}

	// Should show the count.
	if !strings.Contains(output, "(3)") {
		t.Error("expected '(3)' count in view output")
	}

	// Should show all project names.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !strings.Contains(output, name) {
			t.Errorf("expected %q in view output", name)
		}
	}

	// Should show stats for first project.
	if !strings.Contains(output, "2 repos") {
		t.Error("expected '2 repos' for alpha in view output")
	}
	if !strings.Contains(output, "3 PRs") {
		t.Error("expected '3 PRs' for alpha in view output")
	}

	// Selected item should have bullet ●.
	if !strings.Contains(output, "●") {
		t.Error("expected ● bullet for selected item")
	}
}

func TestDashboardView_ViewSelectedHighlight(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()

	// Select each project and verify the selected bullet appears.
	for i := range d.Projects {
		d.Selected = i
		output := d.View()
		if !strings.Contains(output, "●") {
			t.Errorf("expected ● bullet at Selected=%d", i)
		}
		if !strings.Contains(output, d.Projects[i].Name) {
			t.Errorf("expected project name %q in view at Selected=%d", d.Projects[i].Name, i)
		}
	}
}

func TestDashboardView_ViewEmptyProjects(t *testing.T) {
	d := NewDashboardView()
	output := d.View()

	if !strings.Contains(output, "Projects") {
		t.Error("expected 'Projects' title even with 0 projects")
	}
	if !strings.Contains(output, "(0)") {
		t.Error("expected '(0)' count with no projects")
	}
}

func TestDashboardView_ViewShowsSPCHint(t *testing.T) {
	d := NewDashboardView()
	output := d.View()

	if !strings.Contains(output, "SPC") {
		t.Error("expected SPC hint in dashboard view")
	}
}

func TestDashboardView_SingleProject(t *testing.T) {
	d := NewDashboardView()
	d.Projects = []ProjectSummary{
		{Name: "only", RepoCount: 5, PRCount: 2},
	}
	d.Selected = 0

	// j/k should stay at 0.
	d.Update(keyMsg("j"))
	if d.Selected != 0 {
		t.Errorf("j with single project: expected Selected=0, got %d", d.Selected)
	}
	d.Update(keyMsg("k"))
	if d.Selected != 0 {
		t.Errorf("k with single project: expected Selected=0, got %d", d.Selected)
	}

	// G and g also stay at 0.
	d.Update(keyMsg("G"))
	if d.Selected != 0 {
		t.Errorf("G with single project: expected Selected=0, got %d", d.Selected)
	}
	d.Update(keyMsg("g"))
	if d.Selected != 0 {
		t.Errorf("g with single project: expected Selected=0, got %d", d.Selected)
	}
}
