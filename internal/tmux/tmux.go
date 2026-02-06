// Package tmux provides functions to orchestrate tmux panes via exec.
// The app expects to run inside tmux (TMUX env set). Commands target the
// current session automatically.
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

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
