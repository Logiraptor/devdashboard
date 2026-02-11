package ralph

import (
	"fmt"
	"os/exec"
	"strings"
)

// LandingStatus represents the landing state after an agent iteration.
type LandingStatus struct {
	HasUncommittedChanges bool
	HasNewCommit          bool
	BeadClosed            bool
	CommitHashBefore      string
	CommitHashAfter       string
}

// CheckLanding verifies that work was properly "landed" after an agent iteration:
// - No uncommitted changes (or they were committed)
// - Bead is closed
// Returns a LandingStatus and any errors encountered during checking.
func CheckLanding(workDir string, beadID string, beforeCommitHash string) (*LandingStatus, error) {
	status := &LandingStatus{
		CommitHashBefore: beforeCommitHash,
	}

	// Check for uncommitted changes
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	status.HasUncommittedChanges = strings.TrimSpace(string(out)) != ""

	// Get current commit hash
	cmd = exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = workDir
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	status.CommitHashAfter = strings.TrimSpace(string(out))
	status.HasNewCommit = status.CommitHashAfter != "" && status.CommitHashAfter != beforeCommitHash

	// Check if bead is closed (reuse Assess logic)
	assessOutcome, _ := Assess(workDir, beadID, &AgentResult{ExitCode: 0}, nil)
	status.BeadClosed = assessOutcome == OutcomeSuccess

	return status, nil
}

// FormatLandingStatus returns a human-readable summary of landing status.
func FormatLandingStatus(status *LandingStatus) string {
	var parts []string
	if status.HasUncommittedChanges {
		parts = append(parts, "uncommitted changes")
	}
	if !status.HasNewCommit && status.CommitHashBefore != "" {
		parts = append(parts, "no new commit")
	}
	if !status.BeadClosed {
		parts = append(parts, "bead not closed")
	}
	if len(parts) == 0 {
		return "landed successfully"
	}
	return fmt.Sprintf("landing incomplete: %s", strings.Join(parts, ", "))
}
