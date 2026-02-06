package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectSummary holds minimal project info for the dashboard list.
type ProjectSummary struct {
	Name      string
	RepoCount int
	PRCount   int
	Artifacts int
	Selected  bool
}

// DashboardView lists all projects with summaries (Option E).
type DashboardView struct {
	Projects []ProjectSummary
	Selected int
}

// Ensure DashboardView implements View.
var _ View = (*DashboardView)(nil)

// NewDashboardView creates a dashboard with placeholder projects.
func NewDashboardView() *DashboardView {
	return &DashboardView{
		Projects: []ProjectSummary{
			{Name: "HA sampler querier", RepoCount: 2, PRCount: 12, Artifacts: 2, Selected: true},
			{Name: "Project B", RepoCount: 1, PRCount: 5, Artifacts: 1, Selected: false},
			{Name: "Project C", RepoCount: 3, PRCount: 8, Artifacts: 0, Selected: false},
		},
		Selected: 0,
	}
}

// Init implements View.
func (d *DashboardView) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (d *DashboardView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if d.Selected < len(d.Projects)-1 {
				d.Projects[d.Selected].Selected = false
				d.Selected++
				d.Projects[d.Selected].Selected = true
			}
			return d, nil
		case "k", "up":
			if d.Selected > 0 {
				d.Projects[d.Selected].Selected = false
				d.Selected--
				d.Projects[d.Selected].Selected = true
			}
			return d, nil
		case "g":
			// vim: gg = go to top (first key of 'g' sequence; second 'g' would be gg)
			// For now single 'g' goes to top; Phase 4 could add gg
			if d.Selected != 0 {
				d.Projects[d.Selected].Selected = false
				d.Selected = 0
				d.Projects[d.Selected].Selected = true
			}
			return d, nil
		case "G":
			// vim: G = go to bottom
			last := len(d.Projects) - 1
			if last >= 0 && d.Selected != last {
				d.Projects[d.Selected].Selected = false
				d.Selected = last
				d.Projects[d.Selected].Selected = true
			}
			return d, nil
		case "enter":
			return d, nil // Caller handles navigation to detail
		}
	}
	return d, nil
}

// View implements View.
func (d *DashboardView) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("Projects") + " (3)\n")
	b.WriteString(headerStyle.Render("Press [SPC] for commands") + "\n\n")

	for _, p := range d.Projects {
		bullet := "  "
		if p.Selected {
			bullet = "● "
		}
		line := fmt.Sprintf("%s%s  %d repos, %d PRs, %d artifacts",
			bullet, p.Name, p.RepoCount, p.PRCount, p.Artifacts)
		if p.Selected {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(normalStyle.Render(line) + "\n")
		}
	}

	return b.String()
}
