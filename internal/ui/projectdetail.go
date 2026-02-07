package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// ProjectDetailView shows a selected project with resources (repos + PRs) and artifact area.
type ProjectDetailView struct {
	ProjectName   string
	Resources     []project.Resource // unified resource list (repos + PRs)
	Selected      int                // index into Resources for cursor highlight
	PlanContent   string             // from plan.md; empty = "no plan yet"
	DesignContent string             // from design.md; empty = "no design yet"
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
// PlanContent and DesignContent can be set by the caller (from ArtifactStore).
func NewProjectDetailView(name string) *ProjectDetailView {
	return &ProjectDetailView{
		ProjectName:   name,
		Resources:     nil,
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
		case "j", "down":
			if p.Selected < len(p.Resources)-1 {
				p.Selected++
			}
			return p, nil
		case "k", "up":
			if p.Selected > 0 {
				p.Selected--
			}
			return p, nil
		case "g":
			p.Selected = 0
			return p, nil
		case "G":
			if last := len(p.Resources) - 1; last >= 0 {
				p.Selected = last
			}
			return p, nil
		case "esc":
			return p, nil // Caller handles back navigation
		}
	}
	return p, nil
}

// SelectedResource returns a pointer to the currently selected resource, or nil.
func (p *ProjectDetailView) SelectedResource() *project.Resource {
	if p.Selected >= 0 && p.Selected < len(p.Resources) {
		return &p.Resources[p.Selected]
	}
	return nil
}

// View implements View.
func (p *ProjectDetailView) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	sectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	repoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedRepoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	prStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedPRStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	artifactStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)

	var b strings.Builder
	b.WriteString("← " + titleStyle.Render(p.ProjectName) + "\n\n")

	b.WriteString(sectionStyle.Render("Resources") + "\n")
	if len(p.Resources) == 0 {
		b.WriteString("  " + artifactStyle.Render("(no repos added)") + "\n")
	}
	for i, r := range p.Resources {
		selected := i == p.Selected
		status := resourceStatus(r)

		switch r.Kind {
		case project.ResourceRepo:
			bullet := "  "
			if selected {
				bullet = "▸ "
			}
			style := repoStyle
			if selected {
				style = selectedRepoStyle
			}
			b.WriteString(bullet + style.Render(r.RepoName+"/"))
			if status != "" {
				b.WriteString("  " + statusStyle.Render(status))
			}
			b.WriteString("\n")
		case project.ResourcePR:
			if r.PR != nil {
				bullet := "    "
				if selected {
					bullet = "  ▸ "
				}
				state := strings.ToLower(r.PR.State)
				if state == "" {
					state = "open"
				}
				line := fmt.Sprintf("#%d %s (%s)", r.PR.Number, r.PR.Title, state)
				if len(line) > 56 {
					line = line[:53] + "..."
				}
				if selected {
					b.WriteString(bullet + selectedPRStyle.Render(line))
				} else {
					b.WriteString(bullet + prStyle.Render(line))
				}
				if status != "" {
					b.WriteString("  " + statusStyle.Render(status))
				}
				b.WriteString("\n")
			}
		}
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

// resourceStatus returns a status string for display (e.g. "● 2 shells 1 agent").
func resourceStatus(r project.Resource) string {
	if len(r.Panes) == 0 {
		if r.WorktreePath != "" {
			return "●"
		}
		return ""
	}
	shells := 0
	agents := 0
	for _, p := range r.Panes {
		if p.IsAgent {
			agents++
		} else {
			shells++
		}
	}
	var parts []string
	parts = append(parts, "●")
	if shells > 0 {
		parts = append(parts, fmt.Sprintf("%d shell", shells))
		if shells > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	if agents > 0 {
		parts = append(parts, fmt.Sprintf("%d agent", agents))
		if agents > 1 {
			parts[len(parts)-1] += "s"
		}
	}
	return strings.Join(parts, " ")
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
