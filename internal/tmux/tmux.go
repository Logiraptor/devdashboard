// Package tmux provides functions to orchestrate tmux panes via gotmux.
// The app expects to run inside tmux (TMUX env set). Commands target the
// current session automatically.
package tmux

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/GianlucaP106/gotmux/gotmux"
)

// tmuxClient is the package-level gotmux client, initialised lazily.
var tmuxClient *gotmux.Tmux

// client returns the shared gotmux.Tmux instance, creating it on first call.
func client() (*gotmux.Tmux, error) {
	if tmuxClient != nil {
		return tmuxClient, nil
	}
	t, err := gotmux.DefaultTmux()
	if err != nil {
		return nil, fmt.Errorf("init gotmux: %w", err)
	}
	tmuxClient = t
	return tmuxClient, nil
}

// WindowPaneCount returns the number of panes in the current window.
func WindowPaneCount() (int, error) {
	t, err := client()
	if err != nil {
		return 0, err
	}
	out, err := t.Command("display-message", "-p", "#{window_panes}")
	if err != nil {
		return 0, fmt.Errorf("tmux display-message: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
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
	t, err := client()
	if err != nil {
		return err
	}
	if _, err := t.Command("split-window", "-h"); err != nil {
		return fmt.Errorf("tmux split-window: %w", err)
	}
	// split-window focuses the new pane; switch back to devdeploy (left)
	if _, err := t.Command("select-pane", "-L"); err != nil {
		return fmt.Errorf("tmux select-pane: %w", err)
	}
	return nil
}

// SplitPane creates a new pane in the current window with cwd set to workDir.
// Returns the new pane ID (e.g. %4) or an error.
// workDir must be an existing directory; tmux silently ignores bad -c paths.
func SplitPane(workDir string) (paneID string, err error) {
	if info, statErr := os.Stat(workDir); statErr != nil {
		return "", fmt.Errorf("invalid workdir: %w", statErr)
	} else if !info.IsDir() {
		return "", fmt.Errorf("invalid workdir: %s is not a directory", workDir)
	}
	t, err := client()
	if err != nil {
		return "", err
	}
	// gotmux's SplitWindow doesn't return the new pane ID, so use Command
	// with -P -F to print the new pane's ID.
	out, err := t.Command("split-window", "-P", "-F", "#{pane_id}", "-c", workDir)
	if err != nil {
		return "", fmt.Errorf("tmux split-window: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// KillPane kills the pane with the given ID.
func KillPane(paneID string) error {
	t, err := client()
	if err != nil {
		return err
	}
	pane, err := t.GetPaneById(paneID)
	if err != nil {
		return fmt.Errorf("tmux get pane %s: %w", paneID, err)
	}
	if pane == nil {
		return fmt.Errorf("tmux pane %s not found", paneID)
	}
	return pane.Kill()
}

// SendKeys sends keys literally to the pane. Use \n for Enter.
// The -l flag sends keys as typed; newlines are sent as Enter.
func SendKeys(paneID, keys string) error {
	t, err := client()
	if err != nil {
		return err
	}
	// gotmux's Pane.SendKeys omits -l (literal mode), so use Command directly.
	if _, err := t.Command("send-keys", "-l", "-t", paneID, keys); err != nil {
		return fmt.Errorf("tmux send-keys: %w", err)
	}
	return nil
}

// BreakPane moves the pane into its own window (background). Use -d so the new
// window does not become current. The pane ID remains valid for JoinPane.
// break-pane uses -s for source pane; -t is for destination window.
func BreakPane(paneID string) error {
	t, err := client()
	if err != nil {
		return err
	}
	// gotmux has no break-pane wrapper; use Command.
	if _, err := t.Command("break-pane", "-d", "-s", paneID); err != nil {
		return fmt.Errorf("tmux break-pane: %w", err)
	}
	return nil
}

// JoinPane joins the source pane back into the current window. Target "." means
// the current pane (where the app runs). Use -d so focus stays on the app pane.
func JoinPane(paneID string) error {
	t, err := client()
	if err != nil {
		return err
	}
	// gotmux has no join-pane wrapper; use Command.
	if _, err := t.Command("join-pane", "-d", "-s", paneID, "-t", "."); err != nil {
		return fmt.Errorf("tmux join-pane: %w", err)
	}
	return nil
}

// ListPaneIDs returns all live pane IDs across all tmux sessions/windows.
// Each ID looks like "%42". Used for liveness checks by the session tracker.
func ListPaneIDs() (map[string]bool, error) {
	t, err := client()
	if err != nil {
		return nil, err
	}
	panes, err := t.ListAllPanes()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	result := make(map[string]bool, len(panes))
	for _, p := range panes {
		result[p.Id] = true
	}
	return result, nil
}
