package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// reservedChromeLines is the number of terminal lines reserved for app chrome
// (status bar, keybind hints, etc.) that appear outside the detail view.
const reservedChromeLines = 4

// ProjectDetailView shows a selected project with resources (repos + PRs).
type ProjectDetailView struct {
	ProjectName     string
	Resources       []project.Resource // unified resource list (repos + PRs)
	Selected        int                // index into Resources for cursor highlight
	SelectedBeadIdx int                // -1 = resource header, >=0 = bead index within Selected resource
	termWidth       int                // terminal width from WindowSizeMsg; 0 = unknown (use defaults)
	termHeight      int                // terminal height from WindowSizeMsg; 0 = unknown (no scroll)
	viewport        viewport.Model
	contentLines    []string           // cached rendered lines for cursor tracking
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	vp := viewport.New(0, 0)
	return &ProjectDetailView{
		ProjectName:     name,
		Resources:       nil,
		SelectedBeadIdx: -1,
		viewport:        vp,
	}
}

// SetSize updates the terminal dimensions for scroll and truncation calculations.
func (p *ProjectDetailView) SetSize(width, height int) {
	p.termWidth = width
	p.termHeight = height
	vh := p.viewHeight()
	if vh > 0 {
		p.viewport.Width = width
		p.viewport.Height = vh
		p.updateViewportContent()
		p.ensureVisible()
	}
}

// Init implements View.
func (p *ProjectDetailView) Init() tea.Cmd {
	return p.viewport.Init()
}

// Update implements View.
func (p *ProjectDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			p.moveDown()
			p.updateViewportContent()
			p.ensureVisible()
			return p, nil
		case "k", "up":
			p.moveUp()
			p.updateViewportContent()
			p.ensureVisible()
			return p, nil
		case "g":
			p.Selected = 0
			p.SelectedBeadIdx = -1
			p.updateViewportContent()
			p.ensureVisible()
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
			p.updateViewportContent()
			p.ensureVisible()
			return p, nil
		case "esc":
			return p, nil // Caller handles back navigation
		}
	}
	
	// Pass other messages to viewport (mouse wheel, etc.)
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

// maxContentLen returns the maximum number of characters for content text
// at the given visual indent width. When the terminal width is known,
// it subtracts the indent and a suffix margin (for status indicators).
// When the terminal width is unknown (0), fallback is returned.
func (p *ProjectDetailView) maxContentLen(indent, suffixMargin, fallback int) int {
	if p.termWidth <= 0 {
		return fallback
	}
	w := p.termWidth - indent - suffixMargin
	if w < 20 {
		w = 20
	}
	return w
}

// viewHeight returns the number of content lines visible in the viewport.
// Returns 0 if terminal height is unknown (no scrolling).
func (p *ProjectDetailView) viewHeight() int {
	if p.termHeight <= 0 {
		return 0
	}
	h := p.termHeight - reservedChromeLines
	if h < 5 {
		h = 5
	}
	return h
}

// cursorRow returns the 0-based line index of the cursor in the rendered content.
func (p *ProjectDetailView) cursorRow() int {
	// Header: "← title\n\n" (2 lines) + "Resources\n" (1 line) = 3 lines.
	row := 3
	if len(p.Resources) == 0 {
		return row
	}
	for i := 0; i < p.Selected && i < len(p.Resources); i++ {
		row++ // resource header line
		row += len(p.Resources[i].Beads)
	}
	if p.SelectedBeadIdx >= 0 {
		row++ // skip selected resource's header
		row += p.SelectedBeadIdx
	}
	return row
}

// ensureVisible adjusts viewport scroll so the cursor row is within the viewport.
func (p *ProjectDetailView) ensureVisible() {
	if p.viewport.Height <= 0 || len(p.contentLines) == 0 {
		return
	}
	row := p.cursorRow()
	vh := p.viewport.Height
	scrollY := p.viewport.YOffset
	
	// When near the top, snap to 0 to keep the header (title + "Resources") visible.
	// The header occupies the first 3 lines (rows 0-2).
	if row <= 2 {
		p.viewport.SetYOffset(0)
		return
	}
	
	if row < scrollY {
		// Cursor is above viewport, scroll to show cursor at top
		p.viewport.SetYOffset(row)
	} else if row >= scrollY+vh {
		// Cursor is below viewport, scroll to show cursor at bottom
		p.viewport.SetYOffset(row - vh + 1)
	}
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

// updateViewportContent renders the full content and updates the viewport.
func (p *ProjectDetailView) updateViewportContent() {
	content := p.renderContent()
	p.contentLines = strings.Split(content, "\n")
	// Remove trailing empty line from final "\n"
	if len(p.contentLines) > 0 && p.contentLines[len(p.contentLines)-1] == "" {
		p.contentLines = p.contentLines[:len(p.contentLines)-1]
	}
	fullContent := strings.Join(p.contentLines, "\n")
	p.viewport.SetContent(fullContent)
}

// renderContent renders the full project detail content without scrolling.
func (p *ProjectDetailView) renderContent() string {
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
				prMaxLen := p.maxContentLen(4, 24, 56) // bullet(4) + content + status(~24)
				if len(line) > prMaxLen {
					line = line[:prMaxLen-3] + "..."
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

			// Child beads get extra indentation to show hierarchy.
			indent := beadIndent
			if bd.IsChild {
				indent += "  "
			}

			beadLine := bd.ID + "  " + bd.Title
			fallback := 60
			if bd.IsChild {
				fallback = 56 // shorter to compensate for extra indent
			}
			maxLen := p.maxContentLen(len(indent), 18, fallback) // indent + content + status_tag(~18)
			if len(beadLine) > maxLen {
				beadLine = beadLine[:maxLen-3] + "..."
			}
			bullet := indent
			style := beadStyle
			if beadSelected {
				// Replace last 2 chars of indent with "▸ " for selected bead.
				bullet = indent[:len(indent)-2] + "▸ "
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

// View implements View.
func (p *ProjectDetailView) View() string {
	vh := p.viewHeight()
	if vh <= 0 {
		// Terminal size unknown, render without scrolling
		return p.renderContent()
	}
	
	// Always update viewport content to reflect current selection and resources
	// (viewport handles caching internally for performance)
	p.updateViewportContent()
	
	return p.viewport.View()
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

