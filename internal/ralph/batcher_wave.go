package ralph

import (
	"fmt"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
)

// BeadBatcher is a function that yields batches of beads.
// The yield function receives a batch of beads and returns false to stop batching.
type BeadBatcher func(yield func([]beads.Bead) bool)

// fetchReadyBeads fetches all ready beads.
// If epic is set, fetches ready children of that epic.
// Otherwise, fetches all ready beads.
func fetchReadyBeads(workDir string, epic string) ([]beads.Bead, error) {
	if epic != "" {
		// Fetch ready children of epic
		return FetchEpicChildren(nil, workDir, epic)
	}

	// Fetch all ready beads (no epic filter)
	runner := bd.Run
	args := []string{"ready", "--json"}

	out, err := runner(workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return parsed, nil
}

// WaveBatcher yields all ready beads at once, then re-queries for newly unblocked.
func WaveBatcher(workDir string, epic string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		processed := make(map[string]bool)
		for {
			ready, err := fetchReadyBeads(workDir, epic)
			if err != nil {
				// If we can't fetch beads, stop batching
				return
			}
			unprocessed := make([]beads.Bead, 0, len(ready))
			for _, b := range ready {
				if !processed[b.ID] {
					unprocessed = append(unprocessed, b)
				}
			}
			if len(unprocessed) == 0 {
				return
			}
			for _, b := range unprocessed {
				processed[b.ID] = true
			}
			if !yield(unprocessed) {
				return
			}
		}
	}
}
