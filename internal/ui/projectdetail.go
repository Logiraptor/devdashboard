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
	ProjectName     string
	Resources       []project.Resource // unified resource list (repos + PRs)
	Selected        int                // index into Resources for cursor highlight
	SelectedBeadIdx int                // -1 = resource header, >=0 = bead index within Selected resource
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	return &ProjectDetailView{
		ProjectName:     name,
		Resources:       nil,
		SelectedBeadIdx: -1,
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
			p.moveDown()
			return p, nil
		case "k", "up":
			p.moveUp()
			return p, nil
		case "g":
			p.Selected = 0
			p.SelectedBeadIdx = -1
			return p, nil
		case "G":
			if last := len(p.Resources) - 1; last >= 0 {
				p.Selected = last
				if nb := len(p.Resources[last].Beads); nb > 0 {
					p.SelectedBeadIdx = nb - 1
				} else {
					p.SelectedBeadIdx = -1
				}
			}
			return p, nil
		case "esc":
			return p, nil // Caller handles back navigation
		}
	}
	return p, nil
}

// moveDown advances the cursor one row: through beads within a resource,
// then to the next resource header.
func (p *ProjectDetailView) moveDown() {
	if len(p.Resources) == 0 {
		return
	}
	r := &p.Resources[p.Selected]
	// If on header or a bead that's not the last, try to move within this resource's beads.
	if p.SelectedBeadIdx < len(r.Beads)-1 {
		p.SelectedBeadIdx++
		return
	}
	// At last bead (or header with no beads): advance to next resource header.
	if p.Selected < len(p.Resources)-1 {
		p.Selected++
		p.SelectedBeadIdx = -1
	}
}

// moveUp moves the cursor one row up: through beads within a resource,
// then to the previous resource's last bead or header.
func (p *ProjectDetailView) moveUp() {
	if len(p.Resources) == 0 {
		return
	}
	// If on a bead, move up within this resource.
	if p.SelectedBeadIdx > 0 {
		p.SelectedBeadIdx--
		return
	}
	// If on first bead, move to resource header.
	if p.SelectedBeadIdx == 0 {
		p.SelectedBeadIdx = -1
		return
	}
	// On resource header: move to previous resource's last bead (or its header).
	if p.Selected > 0 {
		p.Selected--
		prev := &p.Resources[p.Selected]
		if nb := len(prev.Beads); nb > 0 {
			p.SelectedBeadIdx = nb - 1
		} else {
			p.SelectedBeadIdx = -1
		}
	}
}

// SelectedResource returns a pointer to the currently selected resource, or nil.
func (p *ProjectDetailView) SelectedResource() *project.Resource {
	if p.Selected >= 0 && p.Selected < len(p.Resources) {
		return &p.Resources[p.Selected]
	}
	return nil
}

// SelectedBead returns a pointer to the currently selected bead, or nil if
// the cursor is on a resource header (SelectedBeadIdx == -1) or out of range.
func (p *ProjectDetailView) SelectedBead() *project.BeadInfo {
	r := p.SelectedResource()
	if r == nil || p.SelectedBeadIdx < 0 || p.SelectedBeadIdx >= len(r.Beads) {
		return nil
	}
	return &r.Beads[p.SelectedBeadIdx]
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
	selectedBeadStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	beadStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	dimSelectedRepoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	dimSelectedPRStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	var b strings.Builder
	b.WriteString("← " + titleStyle.Render(p.ProjectName) + "\n\n")

	b.WriteString(sectionStyle.Render("Resources") + "\n")
	if len(p.Resources) == 0 {
		b.WriteString("  " + emptyStyle.Render("(no repos added)") + "\n")
	}
	for i, r := range p.Resources {
		selected := i == p.Selected
		onHeader := selected && p.SelectedBeadIdx == -1
		onChildBead := selected && p.SelectedBeadIdx >= 0
		status := resourceStatus(r)

		// Determine bead indent based on resource kind.
		beadIndent := "    "

		switch r.Kind {
		case project.ResourceRepo:
			bullet := "  "
			if onHeader {
				bullet = "▸ "
			}
			style := repoStyle
			if onHeader {
				style = selectedRepoStyle
			} else if onChildBead {
				style = dimSelectedRepoStyle
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
				if onHeader {
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
				if onHeader {
					b.WriteString(bullet + selectedPRStyle.Render(line))
				} else if onChildBead {
					b.WriteString(bullet + dimSelectedPRStyle.Render(line))
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
		for bi, bd := range r.Beads {
			beadSelected := selected && p.SelectedBeadIdx == bi
			beadLine := bd.ID + "  " + bd.Title
			if len(beadLine) > 60 {
				beadLine = beadLine[:57] + "..."
			}
			bullet := beadIndent
			style := beadStyle
			if beadSelected {
				// Replace last 2 chars of indent with "▸ " for selected bead.
				bullet = beadIndent[:len(beadIndent)-2] + "▸ "
				style = selectedBeadStyle
			}
			rendered := bullet + style.Render(beadLine)
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

