package beads

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Bead represents a bd issue associated with a project resource.
type Bead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
}

// bdListEntry mirrors the JSON shape emitted by `bd list --json`.
type bdListEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
}

// runBD is the function used to execute bd commands.
// Replaced in tests for deterministic output.
var runBD = runBDReal

func runBDReal(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// ListForRepo runs `bd list --label project:<project> --json` in the given
// worktree directory. Returns beads scoped to this project, excluding any
// with a pr:<n> label (those belong to PR resources).
func ListForRepo(worktreeDir, projectName string) []Bead {
	out, err := runBD(worktreeDir,
		"list",
		"--label", fmt.Sprintf("project:%s", projectName),
		"--json",
		"--limit", "0",
	)
	if err != nil {
		return nil
	}
	all := parseBeads(out)

	// Exclude beads that have any pr:* label — those belong to PR resources.
	result := make([]Bead, 0, len(all))
	for _, b := range all {
		if !hasPRLabel(b.Labels) {
			result = append(result, b)
		}
	}
	return result
}

// ListForPR runs `bd list --label project:<project> --label pr:<number> --json`
// in the given worktree directory.
func ListForPR(worktreeDir, projectName string, prNumber int) []Bead {
	out, err := runBD(worktreeDir,
		"list",
		"--label", fmt.Sprintf("project:%s", projectName),
		"--label", fmt.Sprintf("pr:%d", prNumber),
		"--json",
		"--limit", "0",
	)
	if err != nil {
		return nil
	}
	return parseBeads(out)
}

// parseBeads decodes JSON output from bd list into Bead slice.
// Filters to open/in_progress by default (closed beads are noise).
func parseBeads(data []byte) []Bead {
	var entries []bdListEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}

	result := make([]Bead, 0, len(entries))
	for _, e := range entries {
		if e.Status == "closed" {
			continue
		}
		result = append(result, Bead{
			ID:        e.ID,
			Title:     e.Title,
			Status:    e.Status,
			Priority:  e.Priority,
			Labels:    e.Labels,
			CreatedAt: e.CreatedAt,
		})
	}
	return result
}

// hasPRLabel reports whether labels contains a "pr:<number>" label.
func hasPRLabel(labels []string) bool {
	for _, l := range labels {
		if strings.HasPrefix(l, "pr:") {
			return true
		}
	}
	return false
}
