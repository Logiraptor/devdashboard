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
	BeadCount int
	Selected  bool
}

// DashboardView lists all projects with summaries (Option E).
type DashboardView struct {
	Projects []ProjectSummary
	Selected int
}

// Ensure DashboardView implements View.
var _ View = (*DashboardView)(nil)

// NewDashboardView creates a dashboard. Projects are loaded from disk via ProjectsLoadedMsg.
func NewDashboardView() *DashboardView {
	return &DashboardView{
		Projects: nil,
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
				d.Selected++
			}
			return d, nil
		case "k", "up":
			if d.Selected > 0 {
				d.Selected--
			}
			return d, nil
		case "g":
			if d.Selected != 0 {
				d.Selected = 0
			}
			return d, nil
		case "G":
			last := len(d.Projects) - 1
			if last >= 0 && d.Selected != last {
				d.Selected = last
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
	count := len(d.Projects)
	b.WriteString(titleStyle.Render("Projects") + fmt.Sprintf(" (%d)", count) + "\n")
	b.WriteString(headerStyle.Render("Press [SPC] for commands") + "\n\n")

	for i, p := range d.Projects {
		bullet := "  "
		if i == d.Selected {
			bullet = "● "
		}
		
		// Format PR count (show "…" if loading, i.e., -1)
		prCountStr := "…"
		if p.PRCount >= 0 {
			prCountStr = fmt.Sprintf("%d", p.PRCount)
		}
		
		line := fmt.Sprintf("%s%s  %d repos, %s PRs",
			bullet, p.Name, p.RepoCount, prCountStr)
		
		// Format bead count (show only if loaded and > 0)
		if p.BeadCount > 0 {
			line += fmt.Sprintf(", %d beads", p.BeadCount)
		} else if p.BeadCount == -1 {
			// Show loading indicator for beads too
			line += ", … beads"
		}
		
		if i == d.Selected {
			b.WriteString(selectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(normalStyle.Render(line) + "\n")
		}
	}

	return b.String()
}
