package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProjectDetailView shows a selected project with repos, PRs, and artifact area (Option E).
type ProjectDetailView struct {
	ProjectName   string
	Repos         []string
	PRs           []string
	PlanContent   string // from plan.md; empty = "no plan yet"
	DesignContent string // from design.md; empty = "no design yet"
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
// PlanContent and DesignContent can be set by the caller (from ArtifactStore).
func NewProjectDetailView(name string) *ProjectDetailView {
	return &ProjectDetailView{
		ProjectName:   name,
		Repos:         []string{"repo-a", "repo-b"},
		PRs:           []string{"#42 in review", "#41 merged", "#38 open"},
		PlanContent:   "",
		DesignContent: "",
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
		case "esc":
			return p, nil // Caller handles back navigation
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

	planLabel := "Plan"
	if p.PlanContent == "" {
		planLabel = "Plan (no plan yet)"
	}
	designLabel := "Design"
	if p.DesignContent == "" {
		designLabel = "Design (no design yet)"
	}
	b.WriteString("\n" + sectionStyle.Render("Artifacts") + "\n")
	b.WriteString("  " + artifactStyle.Render(planLabel+": ") + artifactContent(p.PlanContent) + "\n")
	b.WriteString("  " + artifactStyle.Render(designLabel+": ") + artifactContent(p.DesignContent) + "\n")

	return b.String()
}

func artifactContent(s string) string {
	if s == "" {
		return "(empty)"
	}
	lines := splitLines(s)
	if len(lines) == 0 {
		return "(empty)"
	}
	first := lines[0]
	if len(first) > 80 {
		first = first[:77] + "..."
	}
	return first
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
