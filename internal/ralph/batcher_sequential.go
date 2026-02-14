package ralph

import (
	"devdeploy/internal/beads"
)

// BeadBatcher is a function type that yields batches of beads.
// The yield function returns false to stop batching, true to continue.
type BeadBatcher func(yield func([]beads.Bead) bool)

// SequentialBatcher yields one bead at a time from bd ready.
// It includes same-bead retry detection to avoid infinite loops when
// the same failed bead is repeatedly returned.
//
// The retry detection matches the logic from handleSameBeadRetry in loop_sequential.go:
// if the picker returns the same bead that was just yielded (indicating a retry),
// it skips that bead and tries the next one. If all beads are skipped, it stops.
func SequentialBatcher(workDir string, epic string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		picker := &BeadPicker{WorkDir: workDir, Epic: epic}
		lastFailedBeadID := ""
		skippedBeads := make(map[string]bool)
		lastYieldedBeadID := ""

		for {
			bead, err := picker.Next()
			if err != nil || bead == nil {
				return
			}

			// Same-bead retry detection: if this is the same bead that just failed,
			// skip it and try the next one. This matches the logic from handleSameBeadRetry
			// in loop_sequential.go.
			if lastFailedBeadID != "" && bead.ID == lastFailedBeadID {
				skippedBeads[bead.ID] = true
				lastFailedBeadID = "" // reset so we don't skip indefinitely

				// Try one more pick; if that's also skipped, stop.
				retryBead, retryErr := picker.Next()
				if retryErr != nil {
					return
				}
				if retryBead == nil {
					return
				}
				if skippedBeads[retryBead.ID] {
					return
				}
				bead = retryBead
			}

			// Also detect if the picker returns the same bead consecutively
			// (this can happen if a bead fails and the picker keeps returning it)
			if lastYieldedBeadID != "" && bead.ID == lastYieldedBeadID {
				skippedBeads[bead.ID] = true

				// Try one more pick; if that's also skipped, stop.
				retryBead, retryErr := picker.Next()
				if retryErr != nil {
					return
				}
				if retryBead == nil {
					return
				}
				if skippedBeads[retryBead.ID] {
					return
				}
				bead = retryBead
			}

			// Yield the bead (single bead in a slice)
			if !yield([]beads.Bead{*bead}) {
				return
			}

			lastYieldedBeadID = bead.ID
		}
	}
}
