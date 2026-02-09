package ralph

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"time"

	"devdeploy/internal/beads"
)

// bdReadyEntry mirrors the JSON shape emitted by `bd ready --json`.
type bdReadyEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
}

// RunBDFunc is the function signature for executing bd commands.
// Accepts a working directory and arguments, returns raw output.
type RunBDFunc func(dir string, args ...string) ([]byte, error)

// runBDReal executes a real bd command.
func runBDReal(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// BeadPicker queries bd for ready beads and selects the next one to work on.
type BeadPicker struct {
	WorkDir string
	Project string
	Epic    string
	Labels  []string

	// RunBD is the function used to execute bd commands.
	// Defaults to runBDReal. Override in tests for deterministic output.
	RunBD RunBDFunc
}

// Next queries `bd ready --json` for available beads, filters by labels,
// sorts by priority (lowest number = highest priority) then by creation date
// (oldest first), and returns the top bead. Returns nil if no beads are available.
func (p *BeadPicker) Next() (*beads.Bead, error) {
	runner := p.RunBD
	if runner == nil {
		runner = runBDReal
	}

	args := []string{"ready", "--json"}
	if p.Project != "" {
		args = append(args, "--label", fmt.Sprintf("project:%s", p.Project))
	}
	if p.Epic != "" {
		args = append(args, "--parent", p.Epic)
	}
	for _, l := range p.Labels {
		args = append(args, "--label", l)
	}

	out, err := runner(p.WorkDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	if len(parsed) == 0 {
		return nil, nil
	}

	// Sort: priority ascending (P0 > P1 > P2), then creation date ascending (oldest first).
	sort.Slice(parsed, func(i, j int) bool {
		if parsed[i].Priority != parsed[j].Priority {
			return parsed[i].Priority < parsed[j].Priority
		}
		return parsed[i].CreatedAt.Before(parsed[j].CreatedAt)
	})

	top := parsed[0]
	return &top, nil
}

// Count returns the total number of ready beads available.
func (p *BeadPicker) Count() (int, error) {
	runner := p.RunBD
	if runner == nil {
		runner = runBDReal
	}

	args := []string{"ready", "--json"}
	if p.Project != "" {
		args = append(args, "--label", fmt.Sprintf("project:%s", p.Project))
	}
	if p.Epic != "" {
		args = append(args, "--parent", p.Epic)
	}
	for _, l := range p.Labels {
		args = append(args, "--label", l)
	}

	out, err := runner(p.WorkDir, args...)
	if err != nil {
		return 0, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return 0, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return len(parsed), nil
}

// parseReadyBeads decodes JSON output from `bd ready --json` into Bead slices.
func parseReadyBeads(data []byte) ([]beads.Bead, error) {
	var entries []bdReadyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	result := make([]beads.Bead, 0, len(entries))
	for _, e := range entries {
		result = append(result, beads.Bead{
			ID:        e.ID,
			Title:     e.Title,
			Status:    e.Status,
			Priority:  e.Priority,
			Labels:    e.Labels,
			CreatedAt: e.CreatedAt,
		})
	}
	return result, nil
}

