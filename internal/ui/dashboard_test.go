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
	d.updateProjects()

	if d.Selected() != 0 {
		t.Fatalf("expected initial Selected=0, got %d", d.Selected())
	}

	// j moves down.
	d.Update(keyMsg("j"))
	if d.Selected() != 1 {
		t.Errorf("after j: expected Selected=1, got %d", d.Selected())
	}
	d.Update(keyMsg("j"))
	if d.Selected() != 2 {
		t.Errorf("after j j: expected Selected=2, got %d", d.Selected())
	}

	// j at bottom stays at bottom.
	d.Update(keyMsg("j"))
	if d.Selected() != 2 {
		t.Errorf("j at bottom: expected Selected=2, got %d", d.Selected())
	}

	// k moves up.
	d.Update(keyMsg("k"))
	if d.Selected() != 1 {
		t.Errorf("after k: expected Selected=1, got %d", d.Selected())
	}

	// k to top, then again stays at 0.
	d.Update(keyMsg("k"))
	if d.Selected() != 0 {
		t.Fatalf("expected Selected=0 after second k, got %d", d.Selected())
	}
	d.Update(keyMsg("k"))
	if d.Selected() != 0 {
		t.Errorf("k at top: expected Selected=0, got %d", d.Selected())
	}
}

func TestDashboardView_GAndShiftG(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()
	d.updateProjects()

	// G jumps to last.
	d.Update(keyMsg("G"))
	if d.Selected() != 2 {
		t.Errorf("after G: expected Selected=2, got %d", d.Selected())
	}

	// g jumps to first.
	d.Update(keyMsg("g"))
	if d.Selected() != 0 {
		t.Errorf("after g: expected Selected=0, got %d", d.Selected())
	}

	// G when already at last is a no-op.
	d.list.Select(2)
	d.Update(keyMsg("G"))
	if d.Selected() != 2 {
		t.Errorf("G at bottom: expected Selected=2, got %d", d.Selected())
	}

	// g when already at first is a no-op.
	d.list.Select(0)
	d.Update(keyMsg("g"))
	if d.Selected() != 0 {
		t.Errorf("g at top: expected Selected=0, got %d", d.Selected())
	}
}

func TestDashboardView_NavigationWithEmptyProjects(t *testing.T) {
	d := NewDashboardView()
	// No projects.

	d.Update(keyMsg("j"))
	if d.Selected() != 0 {
		t.Errorf("j with no projects: expected Selected=0, got %d", d.Selected())
	}
	d.Update(keyMsg("k"))
	if d.Selected() != 0 {
		t.Errorf("k with no projects: expected Selected=0, got %d", d.Selected())
	}
	d.Update(keyMsg("G"))
	if d.Selected() != 0 {
		t.Errorf("G with no projects: expected Selected=0, got %d", d.Selected())
	}
	d.Update(keyMsg("g"))
	if d.Selected() != 0 {
		t.Errorf("g with no projects: expected Selected=0, got %d", d.Selected())
	}
}

func TestDashboardView_DownArrowNavigation(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()
	d.updateProjects()

	d.Update(keyMsg("down"))
	if d.Selected() != 1 {
		t.Errorf("after down: expected Selected=1, got %d", d.Selected())
	}
	d.Update(keyMsg("up"))
	if d.Selected() != 0 {
		t.Errorf("after up: expected Selected=0, got %d", d.Selected())
	}
}

func TestDashboardView_ViewRendersProjectList(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()
	d.updateProjects()
	d.list.Select(0)

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
}

func TestDashboardView_ViewSelectedHighlight(t *testing.T) {
	d := NewDashboardView()
	d.Projects = testProjects()
	d.updateProjects()

	// Select each project and verify it appears in the view.
	for i := range d.Projects {
		d.list.Select(i)
		output := d.View()
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

func TestDashboardView_BeadCountShown(t *testing.T) {
	d := NewDashboardView()
	d.Projects = []ProjectSummary{
		{Name: "alpha", RepoCount: 2, PRCount: 3, BeadCount: 5},
		{Name: "beta", RepoCount: 1, PRCount: 0, BeadCount: 0},
	}
	d.updateProjects()

	output := d.View()

	// alpha has 5 beads — should show in the line
	if !strings.Contains(output, "5 beads") {
		t.Errorf("expected '5 beads' for alpha, got:\n%s", output)
	}
	// beta has 0 beads — should NOT show bead count
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "beta") && strings.Contains(line, "beads") {
			t.Errorf("expected no bead count for beta (0 beads), got: %s", line)
		}
	}
}

func TestDashboardView_SingleProject(t *testing.T) {
	d := NewDashboardView()
	d.Projects = []ProjectSummary{
		{Name: "only", RepoCount: 5, PRCount: 2},
	}
	d.updateProjects()
	d.list.Select(0)

	// j/k should stay at 0.
	d.Update(keyMsg("j"))
	if d.Selected() != 0 {
		t.Errorf("j with single project: expected Selected=0, got %d", d.Selected())
	}
	d.Update(keyMsg("k"))
	if d.Selected() != 0 {
		t.Errorf("k with single project: expected Selected=0, got %d", d.Selected())
	}

	// G and g also stay at 0.
	d.Update(keyMsg("G"))
	if d.Selected() != 0 {
		t.Errorf("G with single project: expected Selected=0, got %d", d.Selected())
	}
	d.Update(keyMsg("g"))
	if d.Selected() != 0 {
		t.Errorf("g with single project: expected Selected=0, got %d", d.Selected())
	}
}
