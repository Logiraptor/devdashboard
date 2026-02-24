package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"devdeploy/internal/project"
	"devdeploy/internal/ui/textutil"
)

type homeItemType int

const (
	homeItemTypeResource homeItemType = iota
	homeItemTypeBead
)

const (
	reservedChromeLines  = 6
	headerHeight         = 2
	activePanesHeight    = 4
	minListHeight        = 6
	minBeadDetailsHeight = 4
	maxBeadDetailsHeight = 12
)

// GlobalPanesGetter returns active panes for rendering.
type GlobalPanesGetter func() []project.PaneInfo

type homeItem struct {
	itemType    homeItemType
	groupIdx    int
	resourceIdx int
	beadIdx     int
	resource    *project.Resource
	bead        *project.BeadInfo

	hasBeads    bool
	isCollapsed bool
}

type homeDelegate struct{}

func (d homeDelegate) Height() int                                      { return 1 }
func (d homeDelegate) Spacing() int                                     { return 0 }
func (d homeDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd          { return nil }
func (d homeDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	title := item.(homeItem).Title()
	if index == m.Index() {
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent)).Render("▌")
		_, _ = fmt.Fprint(w, cursor+" "+title)
	} else {
		_, _ = fmt.Fprint(w, "  "+title)
	}
}

func resourceStatus(r project.Resource) string {
	if len(r.Panes) > 0 {
		return fmt.Sprintf("%d panes", len(r.Panes))
	}
	return ""
}

func prStateColor(state string) string {
	switch state {
	case "merged":
		return ColorHighlight
	case "closed":
		return ColorDanger
	default:
		return ColorSuccess
	}
}

func beadStatusColor(status string) string {
	switch status {
	case "closed":
		return ColorSuccess
	case "in_progress":
		return ColorWarning
	case "blocked":
		return ColorDanger
	default:
		return ColorAccent
	}
}

func statusDot(color string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("●")
}

func formatPaneLine(index int, name string, isAgent bool) string {
	icon := Styles.Muted.Render("❯")
	if isAgent {
		icon = Styles.Status.Render("◆")
	}
	num := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent)).Render(fmt.Sprintf("%d.", index))
	return num + " " + icon + " " + Styles.Normal.Render(name)
}

func (h homeItem) FilterValue() string {
	if h.itemType == homeItemTypeBead && h.bead != nil {
		return h.bead.ID + " " + h.bead.Title
	}
	if h.resource == nil {
		return ""
	}
	switch h.resource.Kind {
	case project.ResourceRepo:
		return h.resource.RepoName
	case project.ResourceWorktree:
		if h.resource.Worktree != nil {
			return h.resource.RepoName + " " + h.resource.Worktree.Branch + " " + h.resource.Worktree.Path
		}
		return h.resource.RepoName
	case project.ResourcePR:
		if h.resource.PR != nil {
			return fmt.Sprintf("%s #%d %s %s", h.resource.RepoName, h.resource.PR.Number, h.resource.PR.Title, h.resource.PR.HeadRefName)
		}
	}
	return h.resource.RepoName
}

func (h homeItem) Title() string {
	if h.itemType == homeItemTypeBead {
		return h.renderBeadTitle()
	}
	return h.renderResourceTitle()
}

func (h homeItem) Description() string {
	return ""
}

func (h homeItem) collapseIndicator() string {
	if !h.hasBeads {
		return " "
	}
	if h.isCollapsed {
		return "▶"
	}
	return "▼"
}

func (h homeItem) renderResourceTitle() string {
	if h.resource == nil {
		return ""
	}
	status := resourceStatus(*h.resource)
	indicator := h.collapseIndicator()
	switch h.resource.Kind {
	case project.ResourceRepo:
		bold := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText)).Bold(true)
		line := bold.Render(h.resource.RepoName + "/")
		if status != "" {
			line += "  " + Styles.Status.Render(status)
		}
		return indicator + " " + line
	case project.ResourceWorktree:
		branch := "(unknown branch)"
		if h.resource.Worktree != nil && h.resource.Worktree.Branch != "" {
			branch = h.resource.Worktree.Branch
		}
		arrow := Styles.Status.Render("↳")
		line := arrow + " " + Styles.Normal.Render(branch)
		if status != "" {
			line += "  " + Styles.Status.Render(status)
		}
		return "  " + indicator + " " + line
	case project.ResourcePR:
		if h.resource.PR == nil {
			return ""
		}
		state := strings.ToLower(h.resource.PR.State)
		if state == "" {
			state = "open"
		}
		dot := statusDot(prStateColor(state))
		title := Styles.Muted.Render(fmt.Sprintf("#%d %s", h.resource.PR.Number, h.resource.PR.Title))
		stateLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(prStateColor(state))).Render(state)
		line := dot + " " + title + " " + stateLabel
		if status != "" {
			line += "  " + Styles.Status.Render(status)
		}
		return "  " + indicator + " " + line
	default:
		return ""
	}
}

func (h homeItem) renderBeadTitle() string {
	if h.bead == nil || h.resource == nil {
		return ""
	}
	indent := "      "
	if h.resource.Kind == project.ResourcePR {
		indent = "        "
	}
	if h.bead.IsChild {
		indent += "  "
	}

	dot := statusDot(beadStatusColor(h.bead.Status))
	beadLine := h.bead.ID + "  " + h.bead.Title
	return indent + dot + " " + Styles.Muted.Render(beadLine)
}

// HomeView is the primary repo-centric homepage showing repos, worktrees, PRs, and beads.
type HomeView struct {
	repoGroups []project.RepoGroup

	list  list.Model
	items []homeItem

	collapsed map[string]bool

	termWidth  int
	termHeight int

	loadingGroups bool
	loadingBeads  bool
	spinner       spinner.Model

	getGlobalPanes GlobalPanesGetter
}

func collapseKey(groupIdx, resourceIdx int) string {
	return fmt.Sprintf("%d:%d", groupIdx, resourceIdx)
}

// Ensure HomeView implements View.
var _ View = (*HomeView)(nil)

func NewHomeView() *HomeView {
	l := list.New(nil, homeDelegate{}, 0, 0)
	l.Title = ""
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted)).Background(lipgloss.NoColor{})
	l.Styles.FilterCursor = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent)).Background(lipgloss.NoColor{})
	l.Styles.DefaultFilterCharacterMatch = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent)).Bold(true)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Styles.Status

	return &HomeView{
		repoGroups:     nil,
		list:           l,
		items:          nil,
		collapsed:      make(map[string]bool),
		loadingGroups:  false,
		loadingBeads:   false,
		spinner:        s,
		getGlobalPanes: nil,
	}
}

func (h *HomeView) Init() tea.Cmd {
	if h.loadingGroups || h.loadingBeads {
		return h.spinner.Tick
	}
	return nil
}

func (h *HomeView) spinnerTickCmd() tea.Cmd {
	if h.loadingGroups || h.loadingBeads {
		return h.spinner.Tick
	}
	return nil
}

func (h *HomeView) SetSize(width, height int) {
	h.termWidth = width
	h.termHeight = height
	vh := h.viewHeight()
	if vh > 0 {
		h.list.SetWidth(width)
		h.list.SetHeight(vh)
	}
}

func (h *HomeView) SetRepoGroups(groups []project.RepoGroup) {
	h.repoGroups = groups
	h.buildItems()
}

func (h *HomeView) RepoGroups() []project.RepoGroup {
	return h.repoGroups
}

func (h *HomeView) Selected() int {
	return h.list.Index()
}

func (h *HomeView) SelectedResource() *project.Resource {
	idx := h.list.Index()
	if idx < 0 || idx >= len(h.items) {
		return nil
	}
	item := h.items[idx]
	if item.groupIdx < 0 || item.groupIdx >= len(h.repoGroups) {
		return nil
	}
	group := h.repoGroups[item.groupIdx]
	if item.resourceIdx < 0 || item.resourceIdx >= len(group.Items) {
		return nil
	}
	return &h.repoGroups[item.groupIdx].Items[item.resourceIdx]
}

func (h *HomeView) SelectedBead() *project.BeadInfo {
	idx := h.list.Index()
	if idx < 0 || idx >= len(h.items) {
		return nil
	}
	item := h.items[idx]
	if item.itemType != homeItemTypeBead || item.beadIdx < 0 {
		return nil
	}
	if item.groupIdx < 0 || item.groupIdx >= len(h.repoGroups) {
		return nil
	}
	group := h.repoGroups[item.groupIdx]
	if item.resourceIdx < 0 || item.resourceIdx >= len(group.Items) {
		return nil
	}
	resource := group.Items[item.resourceIdx]
	if item.beadIdx >= len(resource.Beads) {
		return nil
	}
	return &h.repoGroups[item.groupIdx].Items[item.resourceIdx].Beads[item.beadIdx]
}

func (h *HomeView) IsFiltering() bool {
	return h.list.FilterState() == list.Filtering
}

func (h *HomeView) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.SetSize(msg.Width, msg.Height)
		return h, nil
	case tea.KeyMsg:
		if msg.String() == "esc" && !h.IsFiltering() {
			return h, nil
		}
		if msg.String() == "tab" && !h.IsFiltering() {
			h.toggleCollapse()
			return h, nil
		}
	case spinner.TickMsg:
		if h.loadingGroups || h.loadingBeads {
			var cmd tea.Cmd
			h.spinner, cmd = h.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return h, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	h.list, cmd = h.list.Update(msg)
	h.updateFilterStyles()
	cmds = append(cmds, cmd)
	return h, tea.Batch(cmds...)
}

func (h *HomeView) toggleCollapse() {
	idx := h.list.Index()
	if idx < 0 || idx >= len(h.items) {
		return
	}
	item := h.items[idx]
	gi, ri := item.groupIdx, item.resourceIdx

	if !item.hasBeads && item.itemType == homeItemTypeResource {
		return
	}

	key := collapseKey(gi, ri)
	h.collapsed[key] = !h.collapsed[key]

	h.buildItems()

	// After rebuilding, select the parent resource so the cursor
	// doesn't jump to an unrelated item when collapsing from a bead.
	for i, it := range h.items {
		if it.itemType == homeItemTypeResource && it.groupIdx == gi && it.resourceIdx == ri {
			h.list.Select(i)
			break
		}
	}
}

func (h *HomeView) View() string {
	if h.list.Width() == 0 {
		h.list.SetWidth(80)
	}
	if h.list.Height() == 0 {
		if h.termHeight == 0 {
			h.list.SetHeight(10000)
		} else {
			h.list.SetHeight(20)
		}
	}
	h.updateFilterStyles()

	var b strings.Builder
	b.WriteString(Styles.Title.Render("Home") + "\n\n")

	header := Styles.Section.Render("Workspace Repos")
	if h.loadingGroups || h.loadingBeads {
		header += " " + h.spinner.View()
	}
	b.WriteString(header + "\n")

	if len(h.repoGroups) == 0 {
		b.WriteString("  " + Styles.Empty.Render("(no repos found in workspace)") + "\n")
		return b.String()
	}

	b.WriteString(h.list.View())
	b.WriteString(h.renderActivePanesSection())
	b.WriteString(h.renderBeadDetailsSection())
	return b.String()
}

func (h *HomeView) viewHeight() int {
	if h.termHeight <= 0 {
		return 0
	}
	height := h.termHeight - reservedChromeLines - headerHeight - activePanesHeight - h.beadDetailsAllowedHeight()
	if height < minListHeight {
		height = minListHeight
	}
	return height
}

func (h *HomeView) beadDetailsAllowedHeight() int {
	if h.termHeight <= 0 {
		return minBeadDetailsHeight
	}
	fixedOverhead := reservedChromeLines + headerHeight + activePanesHeight
	baselineTotal := fixedOverhead + minListHeight + minBeadDetailsHeight
	if h.termHeight <= baselineTotal {
		return minBeadDetailsHeight
	}
	extraSpace := h.termHeight - baselineTotal
	beadExtra := extraSpace * 2 / 5 // 40% of extra space goes to bead details
	out := minBeadDetailsHeight + beadExtra
	if out > maxBeadDetailsHeight {
		return maxBeadDetailsHeight
	}
	return out
}

func (h *HomeView) buildItems() {
	h.items = nil
	for gi := range h.repoGroups {
		for ri := range h.repoGroups[gi].Items {
			key := collapseKey(gi, ri)
			beads := h.repoGroups[gi].Items[ri].Beads
			resourceItem := homeItem{
				itemType:    homeItemTypeResource,
				groupIdx:    gi,
				resourceIdx: ri,
				beadIdx:     -1,
				resource:    &h.repoGroups[gi].Items[ri],
				bead:        nil,
				hasBeads:    len(beads) > 0,
				isCollapsed: h.collapsed[key],
			}
			h.items = append(h.items, resourceItem)
			if h.collapsed[key] {
				continue
			}
			for bi := range beads {
				beadItem := homeItem{
					itemType:    homeItemTypeBead,
					groupIdx:    gi,
					resourceIdx: ri,
					beadIdx:     bi,
					resource:    &h.repoGroups[gi].Items[ri],
					bead:        &h.repoGroups[gi].Items[ri].Beads[bi],
				}
				h.items = append(h.items, beadItem)
			}
		}
	}

	listItems := make([]list.Item, len(h.items))
	for i := range h.items {
		listItems[i] = h.items[i]
	}
	h.list.SetItems(listItems)
}

func (h *HomeView) updateFilterStyles() {
	if h.list.FilterState() == list.Filtering {
		h.list.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted)).Background(lipgloss.NoColor{})
		h.list.Styles.FilterCursor = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent)).Background(lipgloss.NoColor{})
	} else {
		h.list.Styles.FilterPrompt = lipgloss.NewStyle().Foreground(lipgloss.NoColor{}).Background(lipgloss.NoColor{})
		h.list.Styles.FilterCursor = lipgloss.NewStyle().Foreground(lipgloss.NoColor{}).Background(lipgloss.NoColor{})
	}
}

func (h *HomeView) renderActivePanesSection() string {
	width := h.termWidth
	if width <= 0 {
		width = 80
	}

	var content strings.Builder
	content.WriteString(Styles.Section.Render("Active Panes") + "\n")

	var activePanes []project.PaneInfo
	if h.getGlobalPanes != nil {
		activePanes = h.getGlobalPanes()
	} else {
		activePanes = h.getOrderedActivePanes()
	}

	if len(activePanes) == 0 {
		content.WriteString("  " + Styles.Muted.Render("(none)") + "\n")
	} else {
		maxPanes := activePanesHeight - 1 // -1 for section header line
		if maxPanes > 9 {
			maxPanes = 9
		}
		for i, pane := range activePanes {
			if i >= maxPanes {
				break
			}
			paneName := h.getPaneDisplayName(pane, i+1)
			content.WriteString("  " + paneName + "\n")
		}
	}

	rendered := content.String()
	return "\n" + lipgloss.Place(width, activePanesHeight, lipgloss.Left, lipgloss.Top, rendered)
}

func (h *HomeView) renderBeadDetailsSection() string {
	width := h.termWidth
	if width <= 0 {
		width = 80
	}
	sectionHeight := h.beadDetailsAllowedHeight()

	var content strings.Builder
	content.WriteString(Styles.Section.Render("Bead Details") + "\n")

	bead := h.SelectedBead()
	if bead == nil {
		content.WriteString("  " + Styles.Muted.Render("(select a bead to see details)") + "\n")
	} else {
		content.WriteString("  " + Styles.Normal.Render(bead.ID+"  "+bead.Title) + "\n")
		dot := statusDot(beadStatusColor(bead.Status))
		statusParts := []string{}
		if bead.Status != "" {
			statusParts = append(statusParts, bead.Status)
		}
		if bead.IssueType != "" {
			statusParts = append(statusParts, bead.IssueType)
		}
		if len(statusParts) > 0 {
			content.WriteString("  " + dot + " " + Styles.Status.Render(strings.Join(statusParts, "  ")) + "\n")
		}

		maxDescLines := sectionHeight - 4
		if maxDescLines < 1 {
			maxDescLines = 1
		}
		if bead.Description != "" {
			descLines := strings.Split(bead.Description, "\n")
			for i, line := range descLines {
				if i >= maxDescLines {
					break
				}
				maxLineWidth := width - 2
				if maxLineWidth > 0 {
					line = textutil.Truncate(line, maxLineWidth)
				}
				content.WriteString("  " + Styles.Normal.Render(line) + "\n")
			}
		}
		if len(bead.Labels) > 0 {
			labelsStr := strings.Join(bead.Labels, ", ")
			maxLabelsWidth := width - 2
			if maxLabelsWidth > 0 {
				labelsStr = textutil.Truncate(labelsStr, maxLabelsWidth)
			}
			content.WriteString("  " + Styles.Muted.Render(labelsStr) + "\n")
		}
	}

	rendered := content.String()
	return "\n" + lipgloss.Place(width, sectionHeight, lipgloss.Left, lipgloss.Top, rendered)
}

func (h *HomeView) getOrderedActivePanes() []project.PaneInfo {
	var panes []project.PaneInfo
	for _, g := range h.repoGroups {
		for _, item := range g.Items {
			panes = append(panes, item.Panes...)
			if len(panes) >= 9 {
				return panes[:9]
			}
		}
	}
	return panes
}

func (h *HomeView) getPaneDisplayName(pane project.PaneInfo, index int) string {
	resourceName := pane.ID
	for _, g := range h.repoGroups {
		for _, item := range g.Items {
			for _, rp := range item.Panes {
				if rp.ID != pane.ID {
					continue
				}
				switch item.Kind {
				case project.ResourcePR:
					if item.PR != nil {
						resourceName = fmt.Sprintf("%s-pr-%d", item.RepoName, item.PR.Number)
					} else {
						resourceName = item.RepoName
					}
				case project.ResourceWorktree:
					if item.Worktree != nil && item.Worktree.Branch != "" {
						resourceName = fmt.Sprintf("%s@%s", item.RepoName, item.Worktree.Branch)
					} else {
						resourceName = item.RepoName
					}
				default:
					resourceName = item.RepoName
				}
				return formatPaneLine(index, resourceName, pane.IsAgent)
			}
		}
	}

	return formatPaneLine(index, resourceName, pane.IsAgent)
}
