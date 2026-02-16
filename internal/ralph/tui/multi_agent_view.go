package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"devdeploy/internal/beads"

	"github.com/charmbracelet/lipgloss"
)

// MultiAgentView displays multiple agent blocks in a grid layout
type MultiAgentView struct {
	agents       map[string]*AgentBlock // keyed by bead ID
	order        []string               // maintains display order
	styles       AgentBlockStyles
	width        int
	height       int
	iterCounter  int
	mu           sync.RWMutex

	// Summary stats
	succeeded int
	failed    int
	questions int
}

// NewMultiAgentView creates a new multi-agent view
func NewMultiAgentView() *MultiAgentView {
	return &MultiAgentView{
		agents: make(map[string]*AgentBlock),
		order:  make([]string, 0),
		styles: DefaultAgentBlockStyles(),
		width:  80,
		height: 24,
	}
}

// SetSize updates the view dimensions
func (v *MultiAgentView) SetSize(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.width = width
	v.height = height
}

// StartAgent begins tracking a new agent
func (v *MultiAgentView) StartAgent(bead beads.Bead) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.iterCounter++
	block := NewAgentBlock(bead, v.iterCounter)
	v.agents[bead.ID] = block
	v.order = append(v.order, bead.ID)
}

// CompleteAgent marks an agent as complete
func (v *MultiAgentView) CompleteAgent(beadID string, status string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if block, ok := v.agents[beadID]; ok {
		block.SetStatus(status)

		// Update summary stats
		switch status {
		case "success":
			v.succeeded++
		case "failed", "timeout":
			v.failed++
		case "question":
			v.questions++
		}
	}
}

// AddToolEvent adds a tool event to an agent's stream
func (v *MultiAgentView) AddToolEvent(beadID string, toolName string, started bool, attrs map[string]string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if block, ok := v.agents[beadID]; ok {
		if started {
			block.AddToolStart(toolName, attrs)
		} else {
			block.AddToolEnd(toolName, attrs, 0)
		}
	}
}

// GetActiveBeadID returns the bead ID of the most recently started running agent
// This is used to route tool events when we don't have explicit bead context
func (v *MultiAgentView) GetActiveBeadID() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// Find the most recent running agent
	for i := len(v.order) - 1; i >= 0; i-- {
		id := v.order[i]
		if block, ok := v.agents[id]; ok && block.Status == "running" {
			return id
		}
	}
	return ""
}

// View renders the multi-agent display
func (v *MultiAgentView) View() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if len(v.agents) == 0 {
		return v.renderWaiting()
	}

	var blocks []string

	// Calculate block width - try to fit 2 columns if wide enough
	cols := 1
	blockWidth := v.width - 2
	if v.width > 120 {
		cols = 2
		blockWidth = (v.width - 4) / 2
	}
	if blockWidth < 50 {
		blockWidth = 50
	}

	// Render all agent blocks
	renderedBlocks := make([]string, 0, len(v.order))
	for _, id := range v.order {
		if block, ok := v.agents[id]; ok {
			rendered := block.Render(v.styles, blockWidth)
			renderedBlocks = append(renderedBlocks, rendered)
		}
	}

	// Arrange in columns
	if cols == 1 {
		blocks = renderedBlocks
	} else {
		// Two-column layout
		for i := 0; i < len(renderedBlocks); i += 2 {
			if i+1 < len(renderedBlocks) {
				// Two blocks side by side
				row := lipgloss.JoinHorizontal(lipgloss.Top,
					renderedBlocks[i],
					"  ",
					renderedBlocks[i+1])
				blocks = append(blocks, row)
			} else {
				// Odd block at the end
				blocks = append(blocks, renderedBlocks[i])
			}
		}
	}

	return strings.Join(blocks, "\n")
}

// renderWaiting shows a waiting state
func (v *MultiAgentView) renderWaiting() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Italic(true)
	return style.Render("  Waiting for agents to start...")
}

// Summary returns a formatted summary line
func (v *MultiAgentView) Summary() string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	running := 0
	for _, block := range v.agents {
		if block.Status == "running" {
			running++
		}
	}

	parts := make([]string, 0, 4)

	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if v.succeeded > 0 {
		parts = append(parts, fmt.Sprintf("%d done", v.succeeded))
	}
	if v.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", v.failed))
	}
	if v.questions > 0 {
		parts = append(parts, fmt.Sprintf("%d questions", v.questions))
	}

	if len(parts) == 0 {
		return "Starting..."
	}
	return strings.Join(parts, " | ")
}

// ActiveCount returns the number of currently running agents
func (v *MultiAgentView) ActiveCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()

	count := 0
	for _, block := range v.agents {
		if block.Status == "running" {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of agents tracked
func (v *MultiAgentView) TotalCount() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.agents)
}

// GetIterNum returns the current iteration number
func (v *MultiAgentView) GetIterNum() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.iterCounter
}

// UpdateDuration updates the elapsed time for running agents (called on tick)
func (v *MultiAgentView) UpdateDuration() {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	for _, block := range v.agents {
		if block.Status == "running" {
			block.Duration = now.Sub(block.StartTime)
		}
	}
}
