// Package tmux provides functions to orchestrate tmux panes via exec.
// The app expects to run inside tmux (TMUX env set). Commands target the
// current session automatically.
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// WindowPaneCount returns the number of panes in the current window.
func WindowPaneCount() (int, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#{window_panes}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("tmux display-message: %w: %s", err, strings.TrimSpace(out.String()))
	}
	n, err := strconv.Atoi(strings.TrimSpace(out.String()))
	if err != nil {
		return 0, fmt.Errorf("parse pane count: %w", err)
	}
	return n, nil
}

// EnsureLayout creates a two-pane layout if it doesn't exist: left = devdeploy, right = project area.
// If the current window has only one pane, splits horizontally to create the right pane.
// Idempotent: does nothing if layout already has 2+ panes.
func EnsureLayout() error {
	count, err := WindowPaneCount()
	if err != nil {
		return err
	}
	if count >= 2 {
		return nil // layout already exists
	}
	cmd := exec.Command("tmux", "split-window", "-h")
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux split-window: %w: %s", err, strings.TrimSpace(out.String()))
	}
	// split-window focuses the new pane; switch back to devdeploy (left)
	cmd = exec.Command("tmux", "select-pane", "-L")
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux select-pane: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// SplitPane creates a new pane in the current window with cwd set to workDir.
// Returns the new pane ID (e.g. %4) or an error.
func SplitPane(workDir string) (paneID string, err error) {
	cmd := exec.Command("tmux", "split-window", "-P", "-F", "#{pane_id}", "-c", workDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux split-window: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// KillPane kills the pane with the given ID.
func KillPane(paneID string) error {
	cmd := exec.Command("tmux", "kill-pane", "-t", paneID)
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-pane: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// SendKeys sends keys literally to the pane. Use \n for Enter.
// The -l flag sends keys as typed; newlines are sent as Enter.
func SendKeys(paneID, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-l", "-t", paneID, keys)
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux send-keys: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// BreakPane moves the pane into its own window (background). Use -d so the new
// window does not become current. The pane ID remains valid for JoinPane.
// break-pane uses -s for source pane; -t is for destination window.
func BreakPane(paneID string) error {
	cmd := exec.Command("tmux", "break-pane", "-d", "-s", paneID)
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux break-pane: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// JoinPane joins the source pane back into the current window. Target "." means
// the current pane (where the app runs). Use -d so focus stays on the app pane.
func JoinPane(paneID string) error {
	cmd := exec.Command("tmux", "join-pane", "-d", "-s", paneID, "-t", ".")
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux join-pane: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// ListPaneIDs returns all live pane IDs across all tmux sessions/windows.
// Each ID looks like "%42". Used for liveness checks by the session tracker.
func ListPaneIDs() (map[string]bool, error) {
	cmd := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w: %s", err, strings.TrimSpace(out.String()))
	}
	panes := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			panes[line] = true
		}
	}
	return panes, nil
}
