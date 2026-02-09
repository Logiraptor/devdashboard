package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// ProjectDetailView shows a selected project with resources (repos + PRs).
type ProjectDetailView struct {
	ProjectName string
	Resources   []project.Resource // unified resource list (repos + PRs)
	Selected    int                // index into Resources for cursor highlight
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	return &ProjectDetailView{
		ProjectName: name,
		Resources:   nil,
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
	emptyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	beadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	beadStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))

	var b strings.Builder
	b.WriteString("← " + titleStyle.Render(p.ProjectName) + "\n\n")

	b.WriteString(sectionStyle.Render("Resources") + "\n")
	if len(p.Resources) == 0 {
		b.WriteString("  " + emptyStyle.Render("(no repos added)") + "\n")
	}
	for i, r := range p.Resources {
		selected := i == p.Selected
		status := resourceStatus(r)

		// Determine bead indent based on resource kind.
		beadIndent := "    "

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
			beadIndent = "      "
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

		// Render beads under the resource.
		for _, bd := range r.Beads {
			beadLine := bd.ID + "  " + bd.Title
			if len(beadLine) > 60 {
				beadLine = beadLine[:57] + "..."
			}
			rendered := beadIndent + beadStyle.Render(beadLine)
			if bd.Status != "" && bd.Status != "open" {
				rendered += "  " + beadStatusStyle.Render("["+bd.Status+"]")
			}
			b.WriteString(rendered + "\n")
		}
	}

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

