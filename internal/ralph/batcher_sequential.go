package ralph

import (
	"devdeploy/internal/beads"
)

// SequentialBatcher yields one bead at a time from bd ready.
// It includes same-bead retry detection to avoid infinite loops when
// the same failed bead is repeatedly returned.
func SequentialBatcher(workDir string, epic string) BeadBatcher {
	return func(yield func([]beads.Bead) bool) {
		skippedBeads := make(map[string]bool)
		lastYieldedBeadID := ""

		for {
			readyBeads, err := ReadyBeads(workDir, epic)
			if err != nil || len(readyBeads) == 0 {
				return
			}

			// Find first non-skipped bead
			var bead *beads.Bead
			for i := range readyBeads {
				if !skippedBeads[readyBeads[i].ID] {
					bead = &readyBeads[i]
					break
				}
			}
			if bead == nil {
				return
			}

			// Detect same-bead retry: if this is the same bead that was just yielded,
			// skip it and try the next one.
			if lastYieldedBeadID != "" && bead.ID == lastYieldedBeadID {
				skippedBeads[bead.ID] = true
				continue
			}

			// Yield the bead (single bead in a slice)
			if !yield([]beads.Bead{*bead}) {
				return
			}

			lastYieldedBeadID = bead.ID
		}
	}
}
