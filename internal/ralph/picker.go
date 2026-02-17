package ralph

import (
	"fmt"
	"sort"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
	"devdeploy/internal/jsonutil"
)

// bdReadyEntry mirrors the JSON shape emitted by `bd ready --json`.
// It embeds BDEntryBase for common fields.
type bdReadyEntry struct {
	beads.BDEntryBase
}

// BDRunner is the function signature for executing bd commands.
type BDRunner = bd.Runner

// ReadyBeads fetches ready beads from bd.
// If parentBead is set, fetches ready children of that epic/parent.
// Otherwise, fetches all ready beads.
// Returns beads sorted by priority (ascending) then creation date (oldest first).
func ReadyBeads(workDir, parentBead string) ([]beads.Bead, error) {
	return ReadyBeadsWithRunner(bd.Run, workDir, parentBead)
}

// ReadyBeadsWithRunner is like ReadyBeads but allows injecting a custom bd runner for testing.
func ReadyBeadsWithRunner(runBD BDRunner, workDir, parentBead string) ([]beads.Bead, error) {
	args := []string{"ready", "--json"}
	if parentBead != "" {
		args = append(args, "--parent", parentBead)
	}

	out, err := runBD(workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	// Sort by priority (ascending) then creation date (oldest first)
	sort.Slice(parsed, func(i, j int) bool {
		if parsed[i].Priority != parsed[j].Priority {
			return parsed[i].Priority < parsed[j].Priority
		}
		return parsed[i].CreatedAt.Before(parsed[j].CreatedAt)
	})

	return parsed, nil
}

// parseReadyBeads decodes JSON output from `bd ready --json` into Bead slices.
func parseReadyBeads(data []byte) ([]beads.Bead, error) {
	entries, err := jsonutil.UnmarshalArrayAllowEmpty[bdReadyEntry](data, "parsing bd ready output")
	if err != nil {
		return nil, err
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
