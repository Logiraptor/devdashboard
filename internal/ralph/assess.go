package ralph

import (
	"encoding/json"
	"fmt"
	"strings"

	"devdeploy/internal/bd"
	"devdeploy/internal/beads"
	"devdeploy/internal/jsonutil"
)

// Outcome represents the result of an agent iteration.
type Outcome int

const (
	OutcomeSuccess  Outcome = iota // Bead was closed by the agent.
	OutcomeQuestion                // Agent created needs-human blocking dependencies.
	OutcomeFailure                 // Agent failed or bead still open with no blockers.
	OutcomeTimeout                 // Agent was killed due to timeout.
)

// String returns a human-readable label for the outcome.
func (o Outcome) String() string {
	switch o {
	case OutcomeSuccess:
		return "success"
	case OutcomeQuestion:
		return "question"
	case OutcomeFailure:
		return "failure"
	case OutcomeTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// parseOutcome converts a string to an Outcome value.
func parseOutcome(s string) (Outcome, error) {
	switch s {
	case "success":
		return OutcomeSuccess, nil
	case "question":
		return OutcomeQuestion, nil
	case "failure":
		return OutcomeFailure, nil
	case "timeout":
		return OutcomeTimeout, nil
	default:
		return 0, ParseEnumError("Outcome", s)
	}
}

// MarshalJSON implements json.Marshaler.
func (o Outcome) MarshalJSON() ([]byte, error) {
	return MarshalEnumJSON(o)
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Outcome) UnmarshalJSON(data []byte) error {
	parsed, err := UnmarshalEnumJSON(data, parseOutcome)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

// bdShowEntry mirrors the JSON shape emitted by `bd show <id> --json`.
// Only the fields we need for assessment are included.
type bdShowEntry struct {
	bdShowBase
	Dependencies []bdShowDep `json:"dependencies"`
	Dependents   []bdShowDep `json:"dependents"`
}

// bdShowDep represents a dependency or dependent in bd show --json output.
type bdShowDep struct {
	ID             string   `json:"id"`
	Status         string   `json:"status"`
	Labels         []string `json:"labels"`
	DependencyType string   `json:"dependency_type"`
}

// BDShowFunc executes bd show for a bead. Nil means use real bd command.
type BDShowFunc func(workDir, beadID string) ([]byte, error)

// Assess evaluates what happened after an agent run. It checks the bead's
// current state via `bd show` and combines that with the AgentResult to
// determine one of four outcomes.
// If bdShow is nil, the real bd command is used.
func Assess(workDir string, beadID string, result *AgentResult, bdShow BDShowFunc) (Outcome, string) {
	// 1. Timeout takes highest priority — the agent didn't finish.
	if result.TimedOut {
		return OutcomeTimeout, fmt.Sprintf(
			"agent timed out after %s (exit code %d)",
			result.Duration.Truncate(1e9), result.ExitCode,
		)
	}

	// 2. Query current bead state.
	if bdShow == nil {
		bdShow = func(dir, id string) ([]byte, error) {
			return bd.Run(dir, "show", id, "--json")
		}
	}
	out, err := bdShow(workDir, beadID)
	if err != nil {
		// Can't determine bead state — treat as failure.
		return OutcomeFailure, fmt.Sprintf(
			"failed to query bead %s (agent exit code %d): %v",
			beadID, result.ExitCode, err,
		)
	}

	entry, err := parseBDShow(out)
	if err != nil {
		return OutcomeFailure, fmt.Sprintf(
			"failed to parse bd show output for %s: %v",
			beadID, err,
		)
	}

	// 3. Success: bead is now closed.
	if entry.Status == beads.StatusClosed {
		return OutcomeSuccess, fmt.Sprintf(
			"bead %s closed successfully (agent ran for %s)",
			beadID, result.Duration.Truncate(1e9),
		)
	}

	// 4. Question: bead still open but has blocking needs-human dependencies.
	if questions := needsHumanDeps(entry); len(questions) > 0 {
		return OutcomeQuestion, fmt.Sprintf(
			"bead %s has %d question(s) needing human input: %s",
			beadID, len(questions), strings.Join(questions, ", "),
		)
	}

	// 5. Failure: bead still open with no question blockers.
	return OutcomeFailure, fmt.Sprintf(
		"bead %s still open after agent run (exit code %d, duration %s)",
		beadID, result.ExitCode, result.Duration.Truncate(1e9),
	)
}

// parseBDShow decodes the JSON array from `bd show <id> --json` and returns
// the first entry. bd show --json always returns a single-element array.
func parseBDShow(data []byte) (*bdShowEntry, error) {
	entries, err := jsonutil.UnmarshalArray[bdShowEntry](data, "parsing bd show output")
	if err != nil {
		return nil, err
	}
	return &entries[0], nil
}

// needsHumanDeps returns the IDs of open dependencies that carry the
// "needs-human" label, indicating the agent raised questions.
func needsHumanDeps(entry *bdShowEntry) []string {
	ids := make([]string, 0, len(entry.Dependencies)+len(entry.Dependents))
	// Check both dependencies and dependents for needs-human beads.
	// Question beads created by the agent will appear as dependencies
	// that block this bead (dependency_type "blocks").
	for _, dep := range entry.Dependencies {
		if dep.Status == beads.StatusClosed {
			continue
		}
		if !hasLabel(dep.Labels, beads.LabelNeedsHuman) {
			continue
		}
		ids = append(ids, dep.ID)
	}
	for _, dep := range entry.Dependents {
		if dep.Status == beads.StatusClosed {
			continue
		}
		if !hasLabel(dep.Labels, beads.LabelNeedsHuman) {
			continue
		}
		ids = append(ids, dep.ID)
	}
	return ids
}

// hasLabel reports whether labels contains the given label.
func hasLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}
