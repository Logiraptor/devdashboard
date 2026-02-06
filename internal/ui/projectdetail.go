package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectDetailView shows a selected project with repos, PRs, and artifact area (Option E).
type ProjectDetailView struct {
	ProjectName string
	Repos       []string
	PRs         []string
	Artifact    string
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	return &ProjectDetailView{
		ProjectName: name,
		Repos:       []string{"repo-a", "repo-b"},
		PRs:         []string{"#42 in review", "#41 merged", "#38 open"},
		Artifact:    "Agent plan (excerpt) — [e] expand, [SPC] open",
	}
}

// Init implements View.
func (p *ProjectDetailView) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (p *ProjectDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			return p, nil // Caller handles back navigation
		case "ctrl+c":
			return p, tea.Quit
		}
	}
	return p, nil
}

// View implements View.
func (p *ProjectDetailView) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	artifactStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)

	var b strings.Builder
	b.WriteString("← " + titleStyle.Render(p.ProjectName) + "\n\n")

	b.WriteString(sectionStyle.Render("Repos") + "\n")
	for _, r := range p.Repos {
		b.WriteString("  • " + normalStyle.Render(r) + "\n")
	}

	b.WriteString("\n" + sectionStyle.Render("PRs / Issues") + "\n")
	for _, pr := range p.PRs {
		b.WriteString("  • " + normalStyle.Render(pr) + "\n")
	}

	b.WriteString("\n" + artifactStyle.Render("Artifact: "+p.Artifact) + "\n")

	return b.String()
}
