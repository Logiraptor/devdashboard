package ralph

import (
	"devdeploy/internal/beads"
)

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
