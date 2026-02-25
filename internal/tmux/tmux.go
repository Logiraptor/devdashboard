// Package tmux provides tmux session and pane helpers used by the UI.
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

// CurrentSessionName returns the tmux session name for the current client.
func CurrentSessionName() (string, error) {
	t, err := client()
	if err != nil {
		return "", err
	}
	out, err := t.Command("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("tmux display-message session_name: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// ListSessionNames returns all live tmux session names.
func ListSessionNames() (map[string]bool, error) {
	t, err := client()
	if err != nil {
		return nil, err
	}
	out, err := t.Command("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	result := make(map[string]bool, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		result[name] = true
	}
	return result, nil
}

// SessionExists reports whether a session exists by name.
func SessionExists(sessionName string) (bool, error) {
	t, err := client()
	if err != nil {
		return false, err
	}
	if _, err := t.Command("has-session", "-t", sessionName); err != nil {
		if strings.Contains(err.Error(), "can't find session") {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session: %w", err)
	}
	return true, nil
}

// EnsureSession creates a detached session if it doesn't already exist.
// Returns created=true when a new session was created.
func EnsureSession(sessionName, workDir, shell string) (created bool, err error) {
	if info, statErr := os.Stat(workDir); statErr != nil {
		return false, fmt.Errorf("invalid workdir: %w", statErr)
	} else if !info.IsDir() {
		return false, fmt.Errorf("invalid workdir: %s is not a directory", workDir)
	}
	if shell == "" {
		shell = "sh"
	}

	t, err := client()
	if err != nil {
		return false, err
	}

	exists, err := SessionExists(sessionName)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := t.Command("new-session", "-d", "-s", sessionName, "-c", workDir, shell); err != nil {
		return false, fmt.Errorf("tmux new-session: %w", err)
	}
	return true, nil
}

// SwitchClient switches the current tmux client to the target session.
func SwitchClient(sessionName string) error {
	t, err := client()
	if err != nil {
		return err
	}
	if _, err := t.Command("switch-client", "-t", sessionName); err != nil {
		return fmt.Errorf("tmux switch-client: %w", err)
	}
	return nil
}

// KillSession kills a tmux session by name.
func KillSession(sessionName string) error {
	t, err := client()
	if err != nil {
		return err
	}
	if _, err := t.Command("kill-session", "-t", sessionName); err != nil {
		return fmt.Errorf("tmux kill-session: %w", err)
	}
	return nil
}

// CaptureOpts controls capture-pane behavior.
type CaptureOpts struct {
	LastLines int // captures this many trailing lines; defaults to full pane when <= 0.
}

// CapturePane returns pane output for session's first pane (session:0.0).
func CapturePane(sessionName string, opts CaptureOpts) (string, error) {
	t, err := client()
	if err != nil {
		return "", err
	}
	target := sessionName + ":0.0"
	args := []string{"capture-pane", "-t", target, "-p"}
	if opts.LastLines > 0 {
		args = append(args, "-S", fmt.Sprintf("-%d", opts.LastLines))
	} else {
		// -S - captures from the start of available history.
		args = append(args, "-S", "-")
	}
	out, err := t.Command(args...)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return strings.TrimRight(out, "\n"), nil
}
