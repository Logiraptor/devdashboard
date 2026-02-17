package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
)

// reservedChromeLines is the number of terminal lines reserved for app chrome
// (status bar, keybind hints, etc.) that appear outside the detail view.
const reservedChromeLines = 4

// Fixed heights for bottom sections to prevent layout jumping when selection changes.
// These sections always render with the same height, padded with empty lines if needed.
const (
	activePanesHeight     = 5  // "Active Panes" header + up to 4 pane entries
	headerHeight          = 3  // Project title + blank line + "Resources" header
	minBeadDetailsHeight  = 5  // Minimum: header + title + status + 1 desc line + labels
	maxBeadDetailsHeight  = 15 // Maximum lines for bead details section
	minListHeight         = 5  // Minimum lines to reserve for the resource list
)


// itemType distinguishes between resource and bead items in the flat list.
type itemType int

const (
	itemTypeResource itemType = iota
	itemTypeBead
)

// detailItem is a unified item type for the flat list (resources + beads).
type detailItem struct {
	itemType    itemType
	resourceIdx int // index into Resources (for both resource and bead items)
	beadIdx     int // -1 for resource items, >=0 for bead items
	resource    *project.Resource
	bead        *project.BeadInfo  // nil for resource items
	view        *ProjectDetailView // reference to view for loading state
	index       int // index in view.items array (for checking selection)
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
	var title string
	if d.itemType == itemTypeResource {
		title = d.renderResourceTitleWithLoading()
	} else {
		title = d.renderBeadTitle()
	}
	
	// Add '▸' cursor prefix if this item is selected
	if d.view != nil && d.index == d.view.list.Index() {
		return "▸ " + title
	}
	return title
}

func (d detailItem) Description() string {
	return ""
}

// renderResourceTitleWithLoading renders the title for a resource item with loading indicators.
func (d detailItem) renderResourceTitleWithLoading() string {
	status := resourceStatusWithLoading(*d.resource, d.view)

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

// GlobalPanesGetter is a function type that returns all active panes globally.
// Used by ProjectDetailView to display panes from all projects, not just the current one.
type GlobalPanesGetter func() []project.PaneInfo

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
	loadingPRs   bool          // true when PRs are being loaded (phase 2)
	loadingBeads bool          // true when beads are being loaded (phase 3)
	spinner      spinner.Model // spinner for loading indicators

	// Global panes access
	getGlobalPanes GlobalPanesGetter // function to get all panes globally; nil falls back to project-only panes
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

	// Set filter styles with theme colors - will be adjusted dynamically in View()
	// Default to hidden (matching background) when not filtering
	l.Styles.FilterPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorMuted)).
		Background(lipgloss.NoColor{})
	l.Styles.FilterCursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)).
		Background(lipgloss.NoColor{})
	l.Styles.DefaultFilterCharacterMatch = lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorAccent)).
		Bold(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Styles.Status

	return &ProjectDetailView{
		ProjectName:  name,
		Resources:    nil,
		list:         l,
		items:        nil,
		itemToIndex:  make(map[int]int),
		loadingPRs:   false,
		loadingBeads: false,
		spinner:      s,
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
	// Start spinner if loading
	if p.loadingPRs || p.loadingBeads {
		return p.spinner.Tick
	}
	return nil
}

// spinnerTickCmd returns a spinner tick command if loading, otherwise nil.
// This is used to start/continue the spinner when loading states change.
func (p *ProjectDetailView) spinnerTickCmd() tea.Cmd {
	if p.loadingPRs || p.loadingBeads {
		return p.spinner.Tick
	}
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
			index:       resourceItemIdx,
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
				index:       beadItemIdx,
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
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	case tea.KeyMsg:
		// When filtering, let the list handle esc/enter to cancel/confirm filter
		// When not filtering, esc is handled by app.go for back navigation
		if msg.String() == "esc" && !p.IsFiltering() {
			// Not filtering - let app.go handle esc for back navigation
			return p, nil
		}
		// When filtering, or for other keys, let it pass through to list
	case spinner.TickMsg:
		// Continue spinner when loading
		if p.loadingPRs || p.loadingBeads {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return p, tea.Batch(cmds...)
	}

	// Pass all messages to list.Model - it handles j/k/g/G navigation and filtering natively
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	cmds = append(cmds, cmd)
	return p, tea.Batch(cmds...)
}

// viewHeight returns the number of content lines visible in the list.
// Returns 0 if terminal height is unknown (no scrolling).
func (p *ProjectDetailView) viewHeight() int {
	if p.termHeight <= 0 {
		return 0
	}
	// Reserve space for: chrome, header, active panes section, bead details section
	h := p.termHeight - reservedChromeLines - headerHeight - activePanesHeight - p.beadDetailsAllowedHeight()
	if h < minListHeight {
		h = minListHeight
	}
	return h
}

// beadDetailsAllowedHeight calculates how many lines the bead details section can use.
// Returns a value between minBeadDetailsHeight and maxBeadDetailsHeight based on terminal size.
// Extra space beyond minimums is split: 40% to bead details, 60% to list.
func (p *ProjectDetailView) beadDetailsAllowedHeight() int {
	if p.termHeight <= 0 {
		return minBeadDetailsHeight
	}

	// Calculate baseline requirement (all fixed sections + minimums)
	fixedOverhead := reservedChromeLines + headerHeight + activePanesHeight
	baselineTotal := fixedOverhead + minListHeight + minBeadDetailsHeight

	// If we don't have enough for baseline, use minimum
	if p.termHeight <= baselineTotal {
		return minBeadDetailsHeight
	}

	// Extra space beyond baseline - give 40% to bead details
	extraSpace := p.termHeight - baselineTotal
	beadExtra := extraSpace * 2 / 5 // 40% of extra goes to bead details

	result := minBeadDetailsHeight + beadExtra
	if result > maxBeadDetailsHeight {
		return maxBeadDetailsHeight
	}
	return result
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

// IsFiltering returns true if the list is currently in filtering mode.
func (p *ProjectDetailView) IsFiltering() bool {
	return p.list.FilterState() == list.Filtering
}

// View implements View.
func (p *ProjectDetailView) View() string {
	// Set default dimensions if not set (for tests)
	if p.list.Width() == 0 {
		p.list.SetWidth(80)
	}
	if p.list.Height() == 0 {
		// If termHeight is 0 (no scrolling), set height to show all items
		// Otherwise, set a reasonable default for tests
		if p.termHeight == 0 {
			// Set height to a very large number to show all items without pagination
			p.list.SetHeight(10000)
		} else {
			p.list.SetHeight(20)
		}
	}

	// Style filter input based on filter state to hide it when not actively filtering
	// This prevents the "blue square" from showing when filter is enabled but not active
	filterState := p.list.FilterState()
	if filterState == list.Filtering {
		// Show filter prompt when actively filtering (user is typing)
		// Use theme colors with no background to avoid blue square
		p.list.Styles.FilterPrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorMuted)).
			Background(lipgloss.NoColor{})
		p.list.Styles.FilterCursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorAccent)).
			Background(lipgloss.NoColor{})
	} else {
		// Hide filter prompt when not actively filtering (Unfiltered or FilterApplied state)
		// Set both foreground and background to transparent/no color to completely hide it
		// This prevents the default blue square from showing when filter is enabled but not active
		p.list.Styles.FilterPrompt = lipgloss.NewStyle().
			Foreground(lipgloss.NoColor{}).
			Background(lipgloss.NoColor{})
		p.list.Styles.FilterCursor = lipgloss.NewStyle().
			Foreground(lipgloss.NoColor{}).
			Background(lipgloss.NoColor{})
	}

	// Rebuild items if Resources have changed or loading state changed
	expectedItems := 0
	for _, r := range p.Resources {
		expectedItems++               // resource item
		expectedItems += len(r.Beads) // bead items
	}
	// Always rebuild items to reflect loading state changes
	if len(p.items) != expectedItems || p.loadingPRs || p.loadingBeads {
		p.buildItems()
	}

	var b strings.Builder
	b.WriteString("← " + Styles.Title.Render(p.ProjectName) + "\n\n")

	// Show spinner next to Resources section when PRs are loading
	resourcesHeader := Styles.Section.Render("Resources")
	if p.loadingPRs {
		resourcesHeader += " " + p.spinner.View()
	}
	b.WriteString(resourcesHeader + "\n")

	if len(p.Resources) == 0 {
		b.WriteString("  " + Styles.Empty.Render("(no repos added)") + "\n")
		return b.String()
	}

	b.WriteString(p.list.View())

	// Render fixed-height bottom sections to prevent layout jumping
	b.WriteString(p.renderActivePanesSection())
	b.WriteString(p.renderBeadDetailsSection())

	return b.String()
}

// renderActivePanesSection renders the Active Panes section with a fixed height.
// Always occupies activePanesHeight lines, padded with empty lines if needed.
func (p *ProjectDetailView) renderActivePanesSection() string {
	width := p.termWidth
	if width <= 0 {
		width = 80
	}

	var content strings.Builder
	content.WriteString(Styles.Section.Render("Active Panes") + "\n")

	var activePanes []project.PaneInfo
	if p.getGlobalPanes != nil {
		activePanes = p.getGlobalPanes()
	} else {
		activePanes = p.getOrderedActivePanes()
	}

	if len(activePanes) == 0 {
		content.WriteString("  " + Styles.Muted.Render("(none)") + "\n")
	} else {
		maxPanes := activePanesHeight - 1 // -1 for header
		if maxPanes > 9 {
			maxPanes = 9
		}
		for i, pane := range activePanes {
			if i >= maxPanes {
				break
			}
			paneName := p.getPaneDisplayName(pane, i+1)
			content.WriteString("  " + paneName + "\n")
		}
	}

	// Pad to fixed height using lipgloss.Place
	rendered := content.String()
	return "\n" + lipgloss.Place(width, activePanesHeight, lipgloss.Left, lipgloss.Top, rendered)
}

// renderBeadDetailsSection renders the Bead Details section with dynamic height.
// Always occupies a consistent height (based on terminal size) to prevent layout jumping.
func (p *ProjectDetailView) renderBeadDetailsSection() string {
	width := p.termWidth
	if width <= 0 {
		width = 80
	}

	sectionHeight := p.beadDetailsAllowedHeight()

	var content strings.Builder
	content.WriteString(Styles.Section.Render("Bead Details") + "\n")

	bead := p.SelectedBead()
	if bead == nil {
		content.WriteString("  " + Styles.Muted.Render("(select a bead to see details)") + "\n")
	} else {
		content.WriteString("  " + Styles.Normal.Render(bead.ID+"  "+bead.Title) + "\n")

		// Status and issue type
		statusParts := []string{}
		if bead.Status != "" {
			statusParts = append(statusParts, bead.Status)
		}
		if bead.IssueType != "" {
			statusParts = append(statusParts, bead.IssueType)
		}
		if len(statusParts) > 0 {
			content.WriteString("  " + Styles.Status.Render(strings.Join(statusParts, "  ")) + "\n")
		}

		// Calculate max description lines: sectionHeight - header(1) - title(1) - status(1) - labels(1)
		maxDescLines := sectionHeight - 4
		if maxDescLines < 1 {
			maxDescLines = 1
		}

		// Description (show as much as will fit)
		if bead.Description != "" {
			descLines := strings.Split(bead.Description, "\n")
			for i, line := range descLines {
				if i >= maxDescLines {
					break
				}
				// Truncate long lines
				if len(line) > width-4 {
					line = line[:width-7] + "..."
				}
				content.WriteString("  " + Styles.Normal.Render(line) + "\n")
			}
		}

		// Labels (if any)
		if len(bead.Labels) > 0 {
			labelsStr := strings.Join(bead.Labels, ", ")
			if len(labelsStr) > width-4 {
				labelsStr = labelsStr[:width-7] + "..."
			}
			content.WriteString("  " + Styles.Muted.Render(labelsStr) + "\n")
		}
	}

	// Pad to consistent height using lipgloss.Place
	rendered := content.String()
	return "\n" + lipgloss.Place(width, sectionHeight, lipgloss.Left, lipgloss.Top, rendered)
}

// resourceStatus returns a status string for display (e.g. "● 2 shells 1 agent").
// If the view is loading beads, shows "…" for bead counts.
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
	// Max 3 parts: 1 header + shells (1) + agents (1)
	parts := make([]string, 0, 3)
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

// resourceStatusWithLoading returns a status string with loading indicators for beads.
func resourceStatusWithLoading(r project.Resource, view *ProjectDetailView) string {
	status := resourceStatus(r)

	// Show "…" for bead counts when beads are loading
	if view != nil && view.loadingBeads && r.WorktreePath != "" {
		beadCount := len(r.Beads)
		if beadCount == 0 {
			// Show loading indicator when beads haven't loaded yet
			if status != "" {
				status += "  …"
			} else {
				status = "…"
			}
		}
	}

	return status
}

// getOrderedActivePanes returns all active panes from Resources, ordered for indexing (1-9).
// Panes are ordered by resource order, then by pane order within each resource.
func (p *ProjectDetailView) getOrderedActivePanes() []project.PaneInfo {
	var allPanes []project.PaneInfo
	for _, r := range p.Resources {
		allPanes = append(allPanes, r.Panes...)
		if len(allPanes) >= 9 {
			// Limit to 9 panes for SPC 1-9
			allPanes = allPanes[:9]
			break
		}
	}
	return allPanes
}

// getPaneDisplayName returns a formatted display name for a pane with its index.
func (p *ProjectDetailView) getPaneDisplayName(pane project.PaneInfo, index int) string {
	// Find which resource this pane belongs to
	var resourceName string
	for _, r := range p.Resources {
		for _, rp := range r.Panes {
			if rp.ID == pane.ID {
				if r.Kind == project.ResourcePR && r.PR != nil {
					resourceName = fmt.Sprintf("%s-pr-%d", r.RepoName, r.PR.Number)
				} else {
					resourceName = r.RepoName
				}
				break
			}
		}
		if resourceName != "" {
			break
		}
	}

	if resourceName == "" {
		resourceName = pane.ID
	}

	paneType := "shell"
	if pane.IsAgent {
		paneType = "agent"
	}

	return fmt.Sprintf("%d. %s (%s)", index, resourceName, paneType)
}
