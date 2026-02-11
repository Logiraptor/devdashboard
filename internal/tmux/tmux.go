// Package tmux provides functions to orchestrate tmux panes via gotmux.
// The app expects to run inside tmux (TMUX env set). Commands target the
// current session automatically.
package tmux

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/GianlucaP106/gotmux/gotmux"
)

var (
	tmuxClient *gotmux.Tmux
	tmuxOnce   sync.Once
	tmuxErr    error
)

// client returns the shared gotmux.Tmux instance, creating it on first call.
// The initialization is thread-safe via sync.Once.
func client() (*gotmux.Tmux, error) {
	tmuxOnce.Do(func() {
		tmuxClient, tmuxErr = gotmux.DefaultTmux()
		if tmuxErr != nil {
			tmuxErr = fmt.Errorf("init gotmux: %w", tmuxErr)
		}
	})
	return tmuxClient, tmuxErr
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
	// Use -h flag for horizontal split (pane opens to the right).
	out, err := t.Command("split-window", "-h", "-P", "-F", "#{pane_id}", "-c", workDir)
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

// FocusPaneAsSidebar brings paneID to the right of the current pane
// and adjusts layout to 50/50 horizontal split.
// If the pane is in a different window, it joins it to the current window.
// The current pane (devdeploy) stays on the left, target pane on the right.
func FocusPaneAsSidebar(paneID string) error {
	t, err := client()
	if err != nil {
		return err
	}

	// Get current pane ID (where devdeploy is running)
	currentPaneID, err := t.Command("display-message", "-p", "#{pane_id}")
	if err != nil {
		return fmt.Errorf("get current pane: %w", err)
	}
	currentPaneID = strings.TrimSpace(currentPaneID)

	// If the target pane is the current pane, nothing to do
	if paneID == currentPaneID {
		return nil
	}

	// Get current window ID
	currentWindowID, err := t.Command("display-message", "-p", "#{window_id}")
	if err != nil {
		return fmt.Errorf("get current window: %w", err)
	}
	currentWindowID = strings.TrimSpace(currentWindowID)

	// Check if pane is already in current window by listing panes
	paneList, err := t.Command("list-panes", "-t", currentWindowID, "-F", "#{pane_id}")
	if err != nil {
		return fmt.Errorf("list panes: %w", err)
	}
	paneIDs := strings.Split(strings.TrimSpace(paneList), "\n")
	paneInWindow := false
	for _, pid := range paneIDs {
		if strings.TrimSpace(pid) == paneID {
			paneInWindow = true
			break
		}
	}

	// If pane is not in current window, join it
	if !paneInWindow {
		// Join the pane horizontally to the right of current pane
		// -h = horizontal split, -s = source pane, -t = target pane
		if _, err := t.Command("join-pane", "-h", "-s", paneID, "-t", currentPaneID); err != nil {
			return fmt.Errorf("join pane: %w", err)
		}
	}

	// Set layout to main-vertical (50/50 horizontal split)
	// This ensures devdeploy on left, selected pane on right
	if _, err := t.Command("select-layout", "-t", currentWindowID, "main-vertical"); err != nil {
		// Fallback to even-vertical if main-vertical fails
		if _, err2 := t.Command("select-layout", "-t", currentWindowID, "even-vertical"); err2 != nil {
			return fmt.Errorf("set layout: %w (tried main-vertical and even-vertical)", err)
		}
	}

	return nil
}
