package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// ProjectSummary holds minimal project info for the dashboard list.
type ProjectSummary struct {
	Name      string
	RepoCount int
	PRCount   int
	BeadCount int
	Selected  bool
}

// projectItem implements list.Item for ProjectSummary.
type projectItem struct {
	ProjectSummary
}

func (p projectItem) FilterValue() string { return p.Name }
func (p projectItem) Title() string {
	// Format PR count (show "…" if loading, i.e., -1)
	prCountStr := "…"
	if p.PRCount >= 0 {
		prCountStr = fmt.Sprintf("%d", p.PRCount)
	}
	
	line := fmt.Sprintf("%s  %d repos, %s PRs", p.Name, p.RepoCount, prCountStr)
	
	// Format bead count (show only if loaded and > 0)
	if p.BeadCount > 0 {
		line += fmt.Sprintf(", %d beads", p.BeadCount)
	} else if p.BeadCount == -1 {
		// Show loading indicator for beads too
		line += ", … beads"
	}
	return line
}
func (p projectItem) Description() string { return "" }

// DashboardView lists all projects with summaries (Option E).
type DashboardView struct {
	list     list.Model
	Projects []ProjectSummary
	spinner  spinner.Model
	loading  bool // true when async enrichment is in progress
}

// Ensure DashboardView implements View.
var _ View = (*DashboardView)(nil)

// NewDashboardView creates a dashboard. Projects are loaded from disk via ProjectsLoadedMsg.
func NewDashboardView() *DashboardView {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	delegate.Styles.NormalDesc = delegate.Styles.NormalTitle
	
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Projects"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	
	return &DashboardView{
		list:     l,
		Projects: nil,
		spinner:  s,
		loading:  false,
	}
}

// Selected returns the index of the currently selected project.
func (d *DashboardView) Selected() int {
	return d.list.Index()
}

// Init implements View.
func (d *DashboardView) Init() tea.Cmd {
	return d.spinner.Tick
}

// SetLoading sets the loading state and returns a command to start/stop spinner.
func (d *DashboardView) SetLoading(loading bool) tea.Cmd {
	d.loading = loading
	if loading {
		return d.spinner.Tick
	}
	return nil
}

// Update implements View.
func (d *DashboardView) Update(msg tea.Msg) (View, tea.Cmd) {
	var cmds []tea.Cmd
	
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.list.SetWidth(msg.Width)
		d.list.SetHeight(msg.Height - 4) // Reserve space for header and hint
		return d, nil
	case spinner.TickMsg:
		if d.loading {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
		return d, tea.Batch(cmds...)
	}
	
	// Pass all messages to list.Model - it handles j/k/g/G navigation natively.
	// Enter is handled by app.go at the application level.
	var cmd tea.Cmd
	d.list, cmd = d.list.Update(msg)
	cmds = append(cmds, cmd)
	return d, tea.Batch(cmds...)
}

// View implements View.
func (d *DashboardView) View() string {
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	
	// Set default dimensions if not set (for tests)
	if d.list.Width() == 0 {
		d.list.SetWidth(80)
	}
	if d.list.Height() == 0 {
		d.list.SetHeight(20)
	}
	
	var b strings.Builder
	count := len(d.Projects)
	title := fmt.Sprintf("Projects (%d)", count)
	if d.loading {
		title += " " + d.spinner.View()
	}
	b.WriteString(title + "\n")
	b.WriteString(headerStyle.Render("Press [SPC] for commands") + "\n\n")
	b.WriteString(d.list.View())
	return b.String()
}

// updateProjects updates the list items from Projects slice.
func (d *DashboardView) updateProjects() {
	items := make([]list.Item, len(d.Projects))
	for i, p := range d.Projects {
		items[i] = projectItem{ProjectSummary: p}
	}
	d.list.SetItems(items)
}
