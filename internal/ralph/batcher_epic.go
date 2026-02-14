package ralph

import (
	"devdeploy/internal/beads"
)

// EpicBatcher yields ready children of an epic one at a time.
// After each yield, it re-queries for newly unblocked children.
func EpicBatcher(workDir string, epicID string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		processed := make(map[string]bool)
		for {
			// Query for ready leaf tasks
			children, err := FetchEpicChildren(nil, workDir, epicID)
			if err != nil {
				// If we can't fetch children, stop batching
				return
			}

			// Filter out already processed beads
			readyChildren := make([]beads.Bead, 0, len(children))
			for _, child := range children {
				if !processed[child.ID] {
					readyChildren = append(readyChildren, child)
				}
			}

			if len(readyChildren) == 0 {
				return
			}

			// Process the first ready leaf (sorted by priority)
			child := readyChildren[0]
			processed[child.ID] = true

			// Yield the bead (single bead in a slice)
			if !yield([]beads.Bead{child}) {
				return
			}
		}
	}
}
