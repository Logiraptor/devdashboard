package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// reservedChromeLines is the number of terminal lines reserved for app chrome
// (status bar, keybind hints, etc.) that appear outside the detail view.
const reservedChromeLines = 4

// itemType distinguishes between resource and bead items in the flat list.
type itemType int

const (
	itemTypeResource itemType = iota
	itemTypeBead
)

// detailItem is a unified item type for the flat list (resources + beads).
type detailItem struct {
	itemType      itemType
	resourceIdx   int // index into Resources (for both resource and bead items)
	beadIdx       int // -1 for resource items, >=0 for bead items
	resource      *project.Resource
	bead          *project.BeadInfo // nil for resource items
	view          *ProjectDetailView // reference to view for loading state
}

func (d detailItem) FilterValue() string {
	if d.itemType == itemTypeResource {
		// Filter by repo name or PR title
		if d.resource.Kind == project.ResourceRepo {
			return d.resource.RepoName
		} else if d.resource.PR != nil {
			return fmt.Sprintf("%s %s", d.resource.PR.Title, d.resource.PR.HeadRefName)
		}
		return ""
	}
	// Filter by bead ID and title
	return d.bead.ID + " " + d.bead.Title
}

func (d detailItem) Title() string {
	if d.itemType == itemTypeResource {
		return d.renderResourceTitleWithLoading()
	}
	return d.renderBeadTitle()
}

func (d detailItem) Description() string {
	return ""
}

// renderResourceTitle renders the title for a resource item.
func (d detailItem) renderResourceTitle() string {
	status := resourceStatus(*d.resource)
	
	switch d.resource.Kind {
	case project.ResourceRepo:
		prefix := "◆ "
		text := d.resource.RepoName + "/"
		if status != "" {
			text += "  " + Styles.Status.Render(status)
		}
		return prefix + Styles.Normal.Render(text)
	case project.ResourcePR:
		if d.resource.PR == nil {
			return ""
		}
		prefix := "◇ "
		state := strings.ToLower(d.resource.PR.State)
		if state == "" {
			state = "open"
		}
		text := fmt.Sprintf("#%d %s (%s)", d.resource.PR.Number, d.resource.PR.Title, state)
		if status != "" {
			text += "  " + Styles.Status.Render(status)
		}
		return prefix + Styles.Muted.Render(text)
	}
	return ""
}

// renderResourceTitleWithLoading renders the title for a resource item with loading indicators.
func (d detailItem) renderResourceTitleWithLoading() string {
	status := resourceStatus(*d.resource)
	
	switch d.resource.Kind {
	case project.ResourceRepo:
		prefix := "◆ "
		text := d.resource.RepoName + "/"
		// Show loading indicator if PRs are being loaded
		if d.view != nil && d.view.loadingPRs {
			text += "  " + Styles.Status.Render("… loading PRs")
		}
		if status != "" {
			text += "  " + Styles.Status.Render(status)
		}
		return prefix + Styles.Normal.Render(text)
	case project.ResourcePR:
		if d.resource.PR == nil {
			return ""
		}
		prefix := "◇ "
		state := strings.ToLower(d.resource.PR.State)
		if state == "" {
			state = "open"
		}
		text := fmt.Sprintf("#%d %s (%s)", d.resource.PR.Number, d.resource.PR.Title, state)
		// Show loading indicator if beads are being loaded
		if d.view != nil && d.view.loadingBeads && d.resource.WorktreePath != "" {
			text += "  " + Styles.Status.Render("… loading beads")
		}
		if status != "" {
			text += "  " + Styles.Status.Render(status)
		}
		return prefix + Styles.Muted.Render(text)
	}
	return ""
}

// renderBeadTitle renders the title for a bead item.
func (d detailItem) renderBeadTitle() string {
	if d.bead == nil {
		return ""
	}
	
	// Determine indentation based on resource kind and bead hierarchy
	indent := "    " // default for repo beads
	if d.resource.Kind == project.ResourcePR {
		indent = "      " // PR beads get more indent
	}
	if d.bead.IsChild {
		indent += "  " // child beads get extra indent
	}
	
	beadLine := d.bead.ID + "  " + d.bead.Title
	beadStatusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))
	
	rendered := indent + Styles.Muted.Render(beadLine)
	if d.bead.Status != "" && d.bead.Status != "open" {
		rendered += "  " + beadStatusStyle.Render("["+d.bead.Status+"]")
	}
	return rendered
}

// ProjectDetailView shows a selected project with resources (repos + PRs).
type ProjectDetailView struct {
	ProjectName string
	Resources   []project.Resource // unified resource list (repos + PRs)
	
	// List-based navigation
	list        list.Model
	items       []detailItem // flat list of resources + beads
	itemToIndex map[int]int  // maps list item index to resource index (for SelectedResource)
	
	termWidth  int // terminal width from WindowSizeMsg; 0 = unknown (use defaults)
	termHeight int // terminal height from WindowSizeMsg; 0 = unknown (no scroll)
	
	// Progressive loading state
	loadingPRs   bool // true when PRs are being loaded (phase 2)
	loadingBeads bool // true when beads are being loaded (phase 3)
}

// Ensure ProjectDetailView implements View.
var _ View = (*ProjectDetailView)(nil)

// NewProjectDetailView creates a detail view for a project.
func NewProjectDetailView(name string) *ProjectDetailView {
	delegate := NewCompactListDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.Title = ""
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	
	return &ProjectDetailView{
		ProjectName:  name,
		Resources:    nil,
		list:         l,
		items:        nil,
		itemToIndex:  make(map[int]int),
		loadingPRs:    false,
		loadingBeads: false,
	}
}

// SetSize updates the terminal dimensions for scroll and truncation calculations.
func (p *ProjectDetailView) SetSize(width, height int) {
	p.termWidth = width
	p.termHeight = height
	vh := p.viewHeight()
	if vh > 0 {
		p.list.SetWidth(width)
		p.list.SetHeight(vh)
	}
}

// Init implements View.
func (p *ProjectDetailView) Init() tea.Cmd {
	return nil
}

// buildItems creates the flat list of items from Resources.
func (p *ProjectDetailView) buildItems() {
	p.items = nil
	p.itemToIndex = make(map[int]int)
	
	for i, r := range p.Resources {
		// Add resource item
		resourceItemIdx := len(p.items)
		p.items = append(p.items, detailItem{
			itemType:    itemTypeResource,
			resourceIdx: i,
			beadIdx:     -1,
			resource:    &p.Resources[i],
			bead:        nil,
			view:        p,
		})
		p.itemToIndex[resourceItemIdx] = i
		
		// Add bead items for this resource
		for bi := range r.Beads {
			beadItemIdx := len(p.items)
			p.items = append(p.items, detailItem{
				itemType:    itemTypeBead,
				resourceIdx: i,
				beadIdx:     bi,
				resource:    &p.Resources[i],
				bead:        &p.Resources[i].Beads[bi],
				view:        p,
			})
			p.itemToIndex[beadItemIdx] = i
		}
	}
	
	// Convert to list.Item slice
	listItems := make([]list.Item, len(p.items))
	for i := range p.items {
		listItems[i] = p.items[i]
	}
	p.list.SetItems(listItems)
}

// Update implements View.
func (p *ProjectDetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	case tea.KeyMsg:
		// Handle esc for back navigation (app.go handles this)
		if msg.String() == "esc" {
			return p, nil
		}
	}
	
	// Pass all messages to list.Model - it handles j/k/g/G navigation and filtering natively
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// viewHeight returns the number of content lines visible in the list.
// Returns 0 if terminal height is unknown (no scrolling).
func (p *ProjectDetailView) viewHeight() int {
	if p.termHeight <= 0 {
		return 0
	}
	h := p.termHeight - reservedChromeLines - 3 // Reserve space for header
	if h < 5 {
		h = 5
	}
	return h
}

// Selected returns the index of the currently selected item in the list.
func (p *ProjectDetailView) Selected() int {
	return p.list.Index()
}

// setSelected sets the selected item index (for testing).
func (p *ProjectDetailView) setSelected(idx int) {
	if idx >= 0 && idx < len(p.items) {
		p.list.Select(idx)
	}
}

// SelectedResource returns a pointer to the currently selected resource, or nil.
// If a bead is selected, returns the resource that contains that bead.
func (p *ProjectDetailView) SelectedResource() *project.Resource {
	idx := p.list.Index()
	if idx < 0 || idx >= len(p.items) {
		return nil
	}
	item := p.items[idx]
	return &p.Resources[item.resourceIdx]
}

// SelectedBead returns a pointer to the currently selected bead, or nil if
// the cursor is on a resource header (not a bead item).
func (p *ProjectDetailView) SelectedBead() *project.BeadInfo {
	idx := p.list.Index()
	if idx < 0 || idx >= len(p.items) {
		return nil
	}
	item := p.items[idx]
	if item.itemType != itemTypeBead || item.beadIdx < 0 {
		return nil
	}
	return &p.Resources[item.resourceIdx].Beads[item.beadIdx]
}

// SelectedResourceIdx returns the index of the currently selected resource.
// If a bead is selected, returns the resource index that contains that bead.
func (p *ProjectDetailView) SelectedResourceIdx() int {
	idx := p.list.Index()
	if idx < 0 || idx >= len(p.items) {
		return -1
	}
	return p.items[idx].resourceIdx
}

// SelectedBeadIdx returns the bead index within the selected resource, or -1
// if the cursor is on a resource header (not a bead item).
func (p *ProjectDetailView) SelectedBeadIdx() int {
	idx := p.list.Index()
	if idx < 0 || idx >= len(p.items) {
		return -1
	}
	item := p.items[idx]
	if item.itemType != itemTypeBead {
		return -1
	}
	return item.beadIdx
}

// View implements View.
func (p *ProjectDetailView) View() string {
	// Set default dimensions if not set (for tests)
	if p.list.Width() == 0 {
		p.list.SetWidth(80)
	}
	if p.list.Height() == 0 {
		p.list.SetHeight(20)
	}
	
	// Rebuild items if Resources have changed or loading state changed
	expectedItems := 0
	for _, r := range p.Resources {
		expectedItems++ // resource item
		expectedItems += len(r.Beads) // bead items
	}
	// Always rebuild items to reflect loading state changes
	if len(p.items) != expectedItems || p.loadingPRs || p.loadingBeads {
		p.buildItems()
	}
	
	var b strings.Builder
	b.WriteString("← " + Styles.Title.Render(p.ProjectName) + "\n\n")
	b.WriteString(Styles.Section.Render("Resources") + "\n")
	
	if len(p.Resources) == 0 {
		b.WriteString("  " + Styles.Empty.Render("(no repos added)") + "\n")
		return b.String()
	}
	
	b.WriteString(p.list.View())
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
