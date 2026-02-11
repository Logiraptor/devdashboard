package ralph

import (
	"fmt"
	"os/exec"
	"strings"

	"devdeploy/internal/beads"
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

	// Check if bead is closed
	closed, err := IsBeadClosed(workDir, beadID, nil)
	if err != nil {
		return nil, fmt.Errorf("checking bead status: %w", err)
	}
	status.BeadClosed = closed

	return status, nil
}

// IsBeadClosed checks if a bead is closed by querying bd.
// If bdShow is nil, the real bd command is used.
func IsBeadClosed(workDir, beadID string, bdShow BDShowFunc) (bool, error) {
	if bdShow == nil {
		bdShow = func(dir, id string) ([]byte, error) {
			cmd := exec.Command("bd", "show", id, "--json")
			cmd.Dir = dir
			return cmd.Output()
		}
	}

	out, err := bdShow(workDir, beadID)
	if err != nil {
		return false, err
	}

	entry, err := parseBDShow(out)
	if err != nil {
		return false, err
	}

	return entry.Status == beads.StatusClosed, nil
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
