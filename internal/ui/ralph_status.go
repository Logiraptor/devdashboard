package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RalphStatus represents the current state of a ralph loop (read from status file).
type RalphStatus struct {
	State         string    `json:"state"`
	Iteration     int       `json:"iteration"`
	MaxIterations int       `json:"max_iterations"`
	CurrentBead   *BeadInfo `json:"current_bead,omitempty"`
	Elapsed       int64     `json:"elapsed_ns"` // nanoseconds
	Tallies       struct {
		Completed int `json:"completed"`
		Questions int `json:"questions"`
		Failed    int `json:"failed"`
		TimedOut  int `json:"timed_out"`
		Skipped   int `json:"skipped"`
	} `json:"tallies"`
	StopReason string `json:"stop_reason,omitempty"`
}

// BeadInfo represents minimal information about a bead.
type BeadInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// RalphStatusMsg is sent when ralph status is polled.
type RalphStatusMsg struct {
	Status *RalphStatus // nil if status file doesn't exist or ralph isn't running
}

// RalphStatusView displays ralph loop progress.
type RalphStatusView struct {
	status *RalphStatus
	width  int
	height int
}

// Ensure RalphStatusView implements View.
var _ View = (*RalphStatusView)(nil)

// NewRalphStatusView creates a new ralph status view.
func NewRalphStatusView() *RalphStatusView {
	return &RalphStatusView{
		status: nil,
		width:  50,
		height: 10,
	}
}

// Init implements View.
func (r *RalphStatusView) Init() tea.Cmd {
	return nil
}

// Update implements View.
func (r *RalphStatusView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case RalphStatusMsg:
		r.status = msg.Status
		return r, nil
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		return r, nil
	}
	return r, nil
}

// View implements View.
func (r *RalphStatusView) View() string {
	if r.status == nil {
		return ""
	}

	var lines []string
	lines = append(lines, Styles.Title.Render("Ralph Loop"))

	// Current iteration
	iterLine := fmt.Sprintf("Iteration: %d/%d", r.status.Iteration, r.status.MaxIterations)
	if r.status.State == "completed" {
		iterLine = fmt.Sprintf("Completed: %d iterations", r.status.Iteration)
	}
	lines = append(lines, iterLine)

	// Current bead
	if r.status.CurrentBead != nil {
		beadLine := fmt.Sprintf("Working on: %s", r.status.CurrentBead.ID)
		if r.status.CurrentBead.Title != "" {
			// Truncate title if too long
			title := r.status.CurrentBead.Title
			maxTitleLen := r.width - len(beadLine) - 10
			if len(title) > maxTitleLen && maxTitleLen > 0 {
				title = title[:maxTitleLen] + "..."
			}
			beadLine += fmt.Sprintf(" - %s", title)
		}
		lines = append(lines, beadLine)
	}

	// Elapsed time
	elapsed := time.Duration(r.status.Elapsed)
	elapsedStr := formatDuration(elapsed)
	lines = append(lines, fmt.Sprintf("Elapsed: %s", elapsedStr))

	// Tallies
	lines = append(lines, "")
	lines = append(lines, "Results:")
	lines = append(lines, fmt.Sprintf("  ✓ %d completed", r.status.Tallies.Completed))
	lines = append(lines, fmt.Sprintf("  ? %d questions", r.status.Tallies.Questions))
	lines = append(lines, fmt.Sprintf("  ✗ %d failed", r.status.Tallies.Failed))
	if r.status.Tallies.TimedOut > 0 {
		lines = append(lines, fmt.Sprintf("  ⏱ %d timed out", r.status.Tallies.TimedOut))
	}
	if r.status.Tallies.Skipped > 0 {
		lines = append(lines, fmt.Sprintf("  ⊘ %d skipped", r.status.Tallies.Skipped))
	}

	// Stop reason (if completed)
	if r.status.State == "completed" && r.status.StopReason != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Stopped: %s", r.status.StopReason))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorHighlight)).
		Padding(1, 2).
		Width(r.width).
		Render(content)

	return box
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
