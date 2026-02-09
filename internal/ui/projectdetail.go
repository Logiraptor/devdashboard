package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
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
	
	// Search state
	searchActive      bool
	searchInput       textinput.Model
	searchMatches     []int // line indices that match the search query
	currentMatchIndex int   // index into searchMatches (-1 = no match selected)
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	vp := viewport.New(0, 0)
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 100
	ti.Width = 40
	return &ProjectDetailView{
		ProjectName:     name,
		Resources:       nil,
		SelectedBeadIdx: -1,
		viewport:        vp,
		searchInput:     ti,
		currentMatchIndex: -1,
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
	// Update search input width
	if p.searchActive && width > 0 {
		p.searchInput.Width = width - 20
		if p.searchInput.Width < 20 {
			p.searchInput.Width = 20
		}
	}
}

// Init implements View.
func (p *ProjectDetailView) Init() tea.Cmd {
	return tea.Batch(p.viewport.Init(), textinput.Blink)
}

// Update implements View.
func (p *ProjectDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	case tea.KeyMsg:
		// Handle search mode
		if p.searchActive {
			// If input is focused, handle search input
			if p.searchInput.Focused() {
				switch msg.String() {
				case "esc":
					p.cancelSearch()
					p.updateViewportContent()
					return p, nil
				case "enter":
					// Accept search and jump to first match, exit input mode
					p.acceptSearch()
					p.searchInput.Blur()
					p.updateViewportContent()
					if len(p.searchMatches) > 0 {
						p.jumpToMatch(0)
					}
					return p, nil
				}
				// Update search input
				var cmd tea.Cmd
				p.searchInput, cmd = p.searchInput.Update(msg)
				query := strings.ToLower(p.searchInput.Value())
				p.updateSearchMatches(query)
				p.updateViewportContent()
				return p, cmd
			} else {
				// Input not focused - search is active but we're navigating matches
				switch msg.String() {
				case "esc":
					p.cancelSearch()
					p.updateViewportContent()
					return p, nil
				case "n":
					// Next match
					if len(p.searchMatches) > 0 {
						p.currentMatchIndex = (p.currentMatchIndex + 1) % len(p.searchMatches)
						p.jumpToMatch(p.currentMatchIndex)
						p.updateViewportContent()
					}
					return p, nil
				case "N":
					// Previous match
					if len(p.searchMatches) > 0 {
						p.currentMatchIndex--
						if p.currentMatchIndex < 0 {
							p.currentMatchIndex = len(p.searchMatches) - 1
						}
						p.jumpToMatch(p.currentMatchIndex)
						p.updateViewportContent()
					}
					return p, nil
				case "/":
					// Start new search
					p.startSearch()
					return p, textinput.Blink
				}
			}
		}
		
		// Normal navigation mode
		switch msg.String() {
		case "/":
			p.startSearch()
			return p, textinput.Blink
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
	repoStyle := Styles.Normal
	selectedRepoStyle := Styles.Selected
	prStyle := Styles.Muted
	selectedPRStyle := Styles.Selected
	dimSelectedRepoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorHighlight))
	dimSelectedPRStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorHighlight))
	beadStyle := Styles.Muted
	selectedBeadStyle := Styles.Selected
	beadStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))

	var b strings.Builder
	b.WriteString("← " + Styles.Title.Render(p.ProjectName) + "\n\n")

	b.WriteString(Styles.Section.Render("Resources") + "\n")
	if len(p.Resources) == 0 {
		b.WriteString("  " + Styles.Empty.Render("(no repos added)") + "\n")
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
				b.WriteString("  " + Styles.Status.Render(status))
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
					b.WriteString("  " + Styles.Status.Render(status))
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
		content := p.renderContent()
		if p.searchActive {
			content += "\n" + p.renderSearchPrompt()
		}
		return content
	}
	
	// Always update viewport content to reflect current selection and resources
	// (viewport handles caching internally for performance)
	p.updateViewportContent()
	
	view := p.viewport.View()
	if p.searchActive {
		view += "\n" + p.renderSearchPrompt()
	}
	return view
}

// startSearch activates search mode.
func (p *ProjectDetailView) startSearch() {
	p.searchActive = true
	p.searchInput.Focus()
	p.searchInput.SetValue("")
	if p.termWidth > 0 {
		p.searchInput.Width = p.termWidth - 20 // Leave room for prompt text
		if p.searchInput.Width < 20 {
			p.searchInput.Width = 20
		}
	}
	p.searchMatches = nil
	p.currentMatchIndex = -1
}

// cancelSearch deactivates search mode.
func (p *ProjectDetailView) cancelSearch() {
	p.searchActive = false
	p.searchInput.Blur()
	p.searchInput.SetValue("")
	p.searchMatches = nil
	p.currentMatchIndex = -1
}

// acceptSearch accepts the current search query and stays in search mode.
func (p *ProjectDetailView) acceptSearch() {
	// Search stays active, just ensure matches are updated
	query := strings.ToLower(p.searchInput.Value())
	p.updateSearchMatches(query)
	if len(p.searchMatches) > 0 {
		p.currentMatchIndex = 0
	} else {
		p.currentMatchIndex = -1
	}
}

// updateSearchMatches finds all lines matching the query.
func (p *ProjectDetailView) updateSearchMatches(query string) {
	p.searchMatches = nil
	p.currentMatchIndex = -1
	
	if query == "" {
		return
	}
	
	queryLower := strings.ToLower(query)
	for i, line := range p.contentLines {
		if strings.Contains(strings.ToLower(line), queryLower) {
			p.searchMatches = append(p.searchMatches, i)
		}
	}
	
	// If we have matches and no current selection, select first match
	if len(p.searchMatches) > 0 && p.currentMatchIndex < 0 {
		p.currentMatchIndex = 0
	}
}

// jumpToMatch navigates to the match at the given index in searchMatches.
func (p *ProjectDetailView) jumpToMatch(matchIdx int) {
	if matchIdx < 0 || matchIdx >= len(p.searchMatches) {
		return
	}
	
	lineIdx := p.searchMatches[matchIdx]
	
	// Convert line index to Selected/SelectedBeadIdx
	// Header: "← title\n\n" (2 lines) + "Resources\n" (1 line) = 3 lines.
	if lineIdx < 3 {
		// In header area, don't move cursor
		return
	}
	
	// Find which resource/bead corresponds to this line
	lineNum := lineIdx - 3 // Skip header lines
	resourceIdx := 0
	beadIdx := -1
	
	for i, r := range p.Resources {
		if lineNum == 0 {
			// This is the resource header
			resourceIdx = i
			beadIdx = -1
			break
		}
		lineNum--
		
		// Check beads
		if lineNum < len(r.Beads) {
			resourceIdx = i
			beadIdx = lineNum
			break
		}
		lineNum -= len(r.Beads)
	}
	
	p.Selected = resourceIdx
	p.SelectedBeadIdx = beadIdx
	p.ensureVisible()
}

// renderSearchPrompt renders the search prompt at the bottom.
func (p *ProjectDetailView) renderSearchPrompt() string {
	matchInfo := ""
	if len(p.searchMatches) > 0 {
		current := p.currentMatchIndex + 1
		if current < 1 {
			current = 1
		}
		matchInfo = fmt.Sprintf(" [%d/%d]", current, len(p.searchMatches))
	} else if p.searchInput.Value() != "" {
		matchInfo = " [no matches]"
	}
	
	var prompt string
	if p.searchInput.Focused() {
		// Input mode - show input with cursor
		inputView := p.searchInput.View()
		prompt = "/" + inputView + matchInfo
		if p.termWidth > 60 {
			prompt += "  Enter: accept  Esc: cancel"
		}
	} else {
		// Navigation mode - show query and navigation hints
		query := p.searchInput.Value()
		if query != "" {
			prompt = "/" + query + matchInfo
		} else {
			prompt = "/" + matchInfo
		}
		if p.termWidth > 60 {
			prompt += "  n: next  N: prev  /: search  Esc: cancel"
		}
	}
	
	return Styles.Hint.Render(prompt)
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

