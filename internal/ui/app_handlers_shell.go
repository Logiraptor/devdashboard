package ui

import (
	"fmt"
	"os/exec"
	"strings"

	"devdeploy/internal/progress"
	"devdeploy/internal/session"
	"devdeploy/internal/tmux"

	tea "github.com/charmbracelet/bubbletea"
)

// handleOpenShell handles OpenShellMsg by opening a shell pane for the selected resource.
func (a *appModelAdapter) handleOpenShell() (tea.Model, tea.Cmd) {
	if a.Mode != ModeProjectDetail || a.Detail == nil {
		return a, nil
	}
	r := a.Detail.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	workDir, err := a.ensureResourceWorktree(r)
	if err != nil {
		a.Status = fmt.Sprintf("Open shell: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	paneID, err := tmux.SplitPane(workDir)
	if err != nil {
		a.Status = fmt.Sprintf("Open shell: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		rk := resourceKeyFromResource(*r)
		a.Sessions.Register(rk, paneID, session.PaneShell)
		a.refreshDetailPanes()
	}
	return a, nil
}

// handleLaunchAgent handles LaunchAgentMsg by launching an agent pane for the selected resource.
func (a *appModelAdapter) handleLaunchAgent() (tea.Model, tea.Cmd) {
	if a.Mode != ModeProjectDetail || a.Detail == nil {
		return a, nil
	}
	r := a.Detail.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	workDir, err := a.ensureResourceWorktree(r)
	if err != nil {
		a.Status = fmt.Sprintf("Launch agent: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	paneID, err := tmux.SplitPane(workDir)
	if err != nil {
		a.Status = fmt.Sprintf("Launch agent: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if err := tmux.SendKeys(paneID, "agent --model claude-4.5-opus-high-thinking --force\n"); err != nil {
		a.Status = fmt.Sprintf("Send agent command: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		rk := resourceKeyFromResource(*r)
		a.Sessions.Register(rk, paneID, session.PaneAgent)
		a.refreshDetailPanes()
	}
	return a, nil
}

// handleLaunchRalph handles LaunchRalphMsg by launching a Ralph loop or agent fallback.
func (a *appModelAdapter) handleLaunchRalph() (tea.Model, tea.Cmd) {
	if a.Mode != ModeProjectDetail || a.Detail == nil {
		return a, nil
	}
	r := a.Detail.SelectedResource()
	if r == nil {
		a.Status = "No resource selected"
		a.StatusIsError = true
		return a, nil
	}
	if len(r.Beads) == 0 {
		a.Status = "No open beads for this resource"
		a.StatusIsError = true
		return a, nil
	}
	workDir, err := a.ensureResourceWorktree(r)
	if err != nil {
		a.Status = fmt.Sprintf("Ralph: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	// Check if ralph binary is available.
	ralphPath, err := exec.LookPath("ralph")
	if err != nil {
		// Fall back to agent-based approach if ralph not found.
		paneID, err := tmux.SplitPane(workDir)
		if err != nil {
			a.Status = fmt.Sprintf("Ralph: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		// Use targeted prompt if cursor is on a specific bead, otherwise use generic prompt.
		prompt := "Run `bd ready` to see available work. Pick one issue, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the rules in .cursor/rules/."
		selectedBead := a.Detail.SelectedBead()
		if selectedBead != nil {
			// Branch to epic-aware flow if the selected bead is an epic
			if selectedBead.IssueType == "epic" {
				prompt = fmt.Sprintf("You are working on epic %s. Run `bd show %s` to understand the epic. Then use `bd ready --parent %s` to find its children. Process them sequentially: for each child, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the rules in .cursor/rules/ and AGENTS.md.", selectedBead.ID, selectedBead.ID, selectedBead.ID)
			} else {
				prompt = fmt.Sprintf("Run `bd show %s` to understand the issue. Claim it with `bd update %s --status in_progress`, implement it, then close it with `bd close %s`. Follow the rules in .cursor/rules/.", selectedBead.ID, selectedBead.ID, selectedBead.ID)
			}
		}
		// Pass the prompt as a single-quoted positional argument to agent.
		// Single quotes prevent the shell from interpreting backticks and $.
		escaped := strings.ReplaceAll(prompt, "'", `'\''`)
		cmd := fmt.Sprintf("agent --model composer-1 --force '%s'\n", escaped)
		if err := tmux.SendKeys(paneID, cmd); err != nil {
			a.Status = fmt.Sprintf("Ralph send agent: %v", err)
			a.StatusIsError = true
			return a, nil
		}
		if a.Sessions != nil {
			rk := resourceKeyFromResource(*r)
			a.Sessions.Register(rk, paneID, session.PaneAgent)
			a.refreshDetailPanes()
		}
		a.Status = "Ralph binary not found, using agent fallback"
		a.StatusIsError = false
		return a, nil
	}
	// Launch ralph binary with --workdir flag.
	// If a specific bead is selected, add --bead flag (or --epic if it's an epic).
	paneID, err := tmux.SplitPane(workDir)
	if err != nil {
		a.Status = fmt.Sprintf("Ralph: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	// Escape the workdir path for shell safety (handle spaces, special chars).
	escapedWorkdir := strings.ReplaceAll(workDir, "'", `'\''`)
	selectedBead := a.Detail.SelectedBead()
	cmd := fmt.Sprintf("%s --workdir '%s'", ralphPath, escapedWorkdir)

	// Always run agents in parallel (3 concurrent by default)
	cmd += " --max-parallel 3"

	if selectedBead != nil {
		escapedBead := strings.ReplaceAll(selectedBead.ID, "'", `'\''`)
		// If selected bead is an epic, use --epic flag for parallel leaf processing
		if selectedBead.IssueType == "epic" {
			cmd += fmt.Sprintf(" --epic '%s'", escapedBead)
		} else {
			cmd += fmt.Sprintf(" --bead '%s'", escapedBead)
		}
	}
	cmd += "\n"
	if err := tmux.SendKeys(paneID, cmd); err != nil {
		a.Status = fmt.Sprintf("Ralph launch: %v", err)
		a.StatusIsError = true
		return a, nil
	}
	if a.Sessions != nil {
		rk := resourceKeyFromResource(*r)
		a.Sessions.Register(rk, paneID, session.PaneAgent)
		a.refreshDetailPanes()
	}
	// User can see ralph output directly in tmux pane
	a.Status = "Ralph loop launched"
	a.StatusIsError = false
	return a, nil
}

// handleHidePane handles HidePaneMsg by hiding the selected resource's latest pane.
func (a *appModelAdapter) handleHidePane() (tea.Model, tea.Cmd) {
	paneID := a.selectedResourceLatestPaneID()
	if paneID == "" {
		a.Status = "No pane to hide"
		return a, nil
	}
	if err := tmux.BreakPane(paneID); err != nil {
		a.Status = fmt.Sprintf("Hide pane: %v", err)
		a.StatusIsError = true
	}
	return a, nil
}

// handleShowPane handles ShowPaneMsg by showing the selected resource's latest pane.
func (a *appModelAdapter) handleShowPane() (tea.Model, tea.Cmd) {
	paneID := a.selectedResourceLatestPaneID()
	if paneID == "" {
		a.Status = "No pane to show"
		return a, nil
	}
	if err := tmux.JoinPane(paneID); err != nil {
		a.Status = fmt.Sprintf("Show pane: %v", err)
		a.StatusIsError = true
	}
	return a, nil
}

// handleFocusPane handles FocusPaneMsg by focusing a pane by index.
func (a *appModelAdapter) handleFocusPane(msg FocusPaneMsg) (tea.Model, tea.Cmd) {
	if a.Sessions == nil {
		a.Status = "No session tracker"
		a.StatusIsError = true
		return a, nil
	}
	// Get ordered list of active panes
	panes := a.getOrderedActivePanes()
	if msg.Index < 1 || msg.Index > len(panes) {
		a.Status = fmt.Sprintf("Pane %d not available (1-%d)", msg.Index, len(panes))
		a.StatusIsError = true
		return a, nil
	}
	// Index is 1-based, convert to 0-based
	pane := panes[msg.Index-1]
	if err := tmux.FocusPaneAsSidebar(pane.PaneID); err != nil {
		a.Status = fmt.Sprintf("Focus pane: %v", err)
		a.StatusIsError = true
	} else {
		// Generate pane name for status
		paneName := a.getPaneDisplayName(pane)
		a.Status = fmt.Sprintf("Focused pane %d: %s", msg.Index, paneName)
		a.StatusIsError = false
	}
	return a, nil
}

// handleTick handles tickMsg by refreshing panes and beads periodically.
func (a *appModelAdapter) handleTick(msg tickMsg) (tea.Model, tea.Cmd) {
	// Periodic refresh: update panes and beads when in project detail mode
	if a.Mode == ModeProjectDetail && a.Detail != nil {
		// Refresh panes (fast, local operation)
		a.refreshDetailPanes()

		// Refresh beads (slower, runs bd command)
		// Only refresh if we have resources with worktrees
		if a.ProjectManager != nil && len(a.Detail.Resources) > 0 {
			hasWorktrees := false
			for _, r := range a.Detail.Resources {
				if r.WorktreePath != "" {
					hasWorktrees = true
					break
				}
			}
			if hasWorktrees {
				return a, tea.Batch(
					loadResourceBeadsCmd(a.Detail.ProjectName, a.Detail.Resources),
					tickCmd(), // Schedule next tick
				)
			}
		}
		return a, tickCmd()
	}
	return a, tickCmd()
}

// handleDismissModal handles DismissModalMsg by dismissing modals, with special handling for progress windows.
func (a *appModelAdapter) handleDismissModal() (tea.Model, tea.Cmd) {
	// If top overlay is ProgressWindow and we have an active agent run, cancel it
	// but keep the overlay visible so the user can see the "Aborted" state.
	// They press Esc again to dismiss after seeing it.
	if a.Overlays.Len() > 0 {
		if top, ok := a.Overlays.Peek(); ok {
			if _, isProgress := top.View.(*ProgressWindow); isProgress && a.agentCancelFunc != nil {
				a.agentCancelFunc()
				a.agentCancelFunc = nil
				return a, nil // Don't pop yet; user will see Aborted, then Esc again to dismiss
			}
		}
	}
	a.Overlays.Pop()
	return a, nil
}

// handleRefresh handles RefreshMsg by clearing PR cache and reloading the current view.
func (a *appModelAdapter) handleRefresh() (tea.Model, tea.Cmd) {
	// Clear PR cache
	if a.ProjectManager != nil {
		a.ProjectManager.ClearPRCache()
	}
	// Reload current view
	if a.Mode == ModeDashboard {
		// Reload dashboard
		return a, loadProjectsCmd(a.ProjectManager)
	} else if a.Mode == ModeProjectDetail && a.Detail != nil && a.ProjectManager != nil {
		// Reload project detail
		return a, loadProjectDetailResourcesCmd(a.ProjectManager, a.Detail.ProjectName)
	}
	return a, nil
}

// handleProgressEvent handles progress.Event by updating progress windows and clearing cancel func.
func (a *appModelAdapter) handleProgressEvent(msg progress.Event) (tea.Model, tea.Cmd) {
	// Run finished (done or aborted); clear cancel so Esc just dismisses
	if msg.Status == progress.StatusDone || msg.Status == progress.StatusAborted {
		a.agentCancelFunc = nil
	}
	if a.Overlays.Len() > 0 {
		if top, hasOverlay := a.Overlays.Peek(); hasOverlay {
			if _, isProgress := top.View.(*ProgressWindow); isProgress {
				if cmd, updated := a.Overlays.UpdateTop(msg); updated {
					return a, cmd
				}
			}
		}
	}
	return a, nil
}
