package ralph

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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

// bdShowEntry mirrors the JSON shape emitted by `bd show <id> --json`.
// Only the fields we need for assessment are included.
type bdShowEntry struct {
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Dependencies []bdShowDep    `json:"dependencies"`
	Dependents   []bdShowDep    `json:"dependents"`
}

// bdShowDep represents a dependency or dependent in bd show --json output.
type bdShowDep struct {
	ID             string   `json:"id"`
	Status         string   `json:"status"`
	Labels         []string `json:"labels"`
	DependencyType string   `json:"dependency_type"`
}

// runBDShow is the function used to execute `bd show <id> --json`.
// Replaced in tests for deterministic output.
var runBDShow = runBDShowReal

func runBDShowReal(workDir string, beadID string) ([]byte, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	cmd.Dir = workDir
	return cmd.Output()
}

// Assess evaluates what happened after an agent run. It checks the bead's
// current state via `bd show` and combines that with the AgentResult to
// determine one of four outcomes.
func Assess(workDir string, beadID string, result *AgentResult) (Outcome, string) {
	// 1. Timeout takes highest priority — the agent didn't finish.
	if result.TimedOut {
		return OutcomeTimeout, fmt.Sprintf(
			"agent timed out after %s (exit code %d)",
			result.Duration.Truncate(1e9), result.ExitCode,
		)
	}

	// 2. Query current bead state.
	out, err := runBDShow(workDir, beadID)
	if err != nil {
		// Can't determine bead state — treat as failure.
		return OutcomeFailure, fmt.Sprintf(
			"failed to query bead %s: %v (agent exit code %d)",
			beadID, err, result.ExitCode,
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
	if entry.Status == "closed" {
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
	var entries []bdShowEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty result from bd show")
	}
	return &entries[0], nil
}

// needsHumanDeps returns the IDs of open dependencies that carry the
// "needs-human" label, indicating the agent raised questions.
func needsHumanDeps(entry *bdShowEntry) []string {
	var ids []string
	// Check both dependencies and dependents for needs-human beads.
	// Question beads created by the agent will appear as dependencies
	// that block this bead (dependency_type "blocks").
	for _, dep := range entry.Dependencies {
		if dep.Status != "closed" && hasLabel(dep.Labels, "needs-human") {
			ids = append(ids, dep.ID)
		}
	}
	for _, dep := range entry.Dependents {
		if dep.Status != "closed" && hasLabel(dep.Labels, "needs-human") {
			ids = append(ids, dep.ID)
		}
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
