package ralph

import (
	"devdeploy/internal/beads"
)

// BeadBatcher is a function type that yields batches of beads.
// The yield function takes a slice of beads and returns true to continue,
// false to stop batching.
type BeadBatcher func(yield func([]beads.Bead) bool)

// TargetedBatcher yields a single specified bead once.
func TargetedBatcher(workDir string, beadID string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		bead, err := fetchTargetBead(workDir, beadID)
		if err != nil || bead == nil {
			return
		}
		yield([]beads.Bead{*bead})
	}
}
