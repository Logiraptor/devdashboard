package ralph

import (
	"devdeploy/internal/beads"
)

// WaveBatcher yields all ready beads at once, then re-queries for newly unblocked.
func WaveBatcher(workDir string, epic string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		processed := make(map[string]bool)
		for {
			ready, err := ReadyBeads(workDir, epic)
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
