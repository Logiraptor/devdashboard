package ralph

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"

	"devdeploy/internal/beads"
)

// bdReadyEntry mirrors the JSON shape emitted by `bd ready --json`.
type bdReadyEntry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Priority  int       `json:"priority"`
	Labels    []string  `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
}

// RunBDFunc is the function signature for executing bd commands.
// Accepts a working directory and arguments, returns raw output.
type RunBDFunc func(dir string, args ...string) ([]byte, error)

// runBDReal executes a real bd command.
func runBDReal(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = dir
	return cmd.Output()
}

// BeadScorer is a function that sorts beads by their score.
// Lower scores indicate higher priority (beads are sorted ascending).
// The scorer receives a slice of beads and sorts them in-place.
type BeadScorer func(beads []beads.Bead)

// BeadPicker queries bd for ready beads and selects the next one to work on.
// BeadPicker is safe for concurrent use.
type BeadPicker struct {
	WorkDir string
	Epic    string
	Labels  []string

	// RunBD is the function used to execute bd commands.
	// Defaults to runBDReal. Override in tests for deterministic output.
	RunBD RunBDFunc

	// Scorer is the function used to sort beads by priority.
	// Defaults to DefaultScorer (priority + creation date).
	// Override to use alternative scoring heuristics.
	Scorer BeadScorer

	// mu protects concurrent access to bd ready queries.
	mu sync.Mutex
}

// Next queries `bd ready --json` for available beads, filters by labels,
// sorts by priority (lowest number = highest priority) then by creation date
// (oldest first), and returns the top bead. Returns nil if no beads are available.
// Next is safe for concurrent use.
func (p *BeadPicker) Next() (*beads.Bead, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	runner := p.RunBD
	if runner == nil {
		runner = runBDReal
	}

	args := []string{"ready", "--json"}
	if p.Epic != "" {
		args = append(args, "--parent", p.Epic)
	}
	for _, l := range p.Labels {
		args = append(args, "--label", l)
	}

	out, err := runner(p.WorkDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	if len(parsed) == 0 {
		return nil, nil
	}

	// Use scorer if provided, otherwise use default.
	scorer := p.Scorer
	if scorer == nil {
		scorer = DefaultScorer
	}

	// Sort beads using the scorer.
	scorer(parsed)

	top := parsed[0]
	return &top, nil
}

// Count returns the total number of ready beads available.
// Count is safe for concurrent use.
func (p *BeadPicker) Count() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	runner := p.RunBD
	if runner == nil {
		runner = runBDReal
	}

	args := []string{"ready", "--json"}
	if p.Epic != "" {
		args = append(args, "--parent", p.Epic)
	}
	for _, l := range p.Labels {
		args = append(args, "--label", l)
	}

	out, err := runner(p.WorkDir, args...)
	if err != nil {
		return 0, fmt.Errorf("bd ready: %w", err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return 0, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return len(parsed), nil
}

// parseReadyBeads decodes JSON output from `bd ready --json` into Bead slices.
func parseReadyBeads(data []byte) ([]beads.Bead, error) {
	var entries []bdReadyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	result := make([]beads.Bead, 0, len(entries))
	for _, e := range entries {
		result = append(result, beads.Bead{
			ID:        e.ID,
			Title:     e.Title,
			Status:    e.Status,
			Priority:  e.Priority,
			Labels:    e.Labels,
			CreatedAt: e.CreatedAt,
		})
	}
	return result, nil
}

// DefaultScorer sorts beads by priority (ascending, lower = higher priority)
// then by creation date (ascending, oldest first).
// This maintains the current behavior of BeadPicker.
func DefaultScorer(beads []beads.Bead) {
	sort.Slice(beads, func(i, j int) bool {
		if beads[i].Priority != beads[j].Priority {
			return beads[i].Priority < beads[j].Priority
		}
		return beads[i].CreatedAt.Before(beads[j].CreatedAt)
	})
}

// ComplexityScorer sorts beads using complexity estimation heuristics:
// - Prefers beads with longer titles (more context/specification)
// - Prefers beads with more labels (more metadata/context)
// - Within same complexity, uses priority as tiebreaker
// - Finally sorts by creation date (oldest first)
//
// This scorer favors well-specified beads that are likely to be
// easier to work on due to having more context.
func ComplexityScorer(beads []beads.Bead) {
	sort.Slice(beads, func(i, j int) bool {
		bi, bj := beads[i], beads[j]

		// Calculate complexity scores (lower = simpler = higher priority)
		// Title length: longer titles = more context = lower complexity score
		titleScoreI := -len(bi.Title)
		titleScoreJ := -len(bj.Title)

		// Label count: more labels = more context = lower complexity score
		labelScoreI := -len(bi.Labels)
		labelScoreJ := -len(bj.Labels)

		// Combined complexity score
		complexityI := titleScoreI + labelScoreI
		complexityJ := titleScoreJ + labelScoreJ

		// Sort by complexity (lower score = simpler = higher priority)
		if complexityI != complexityJ {
			return complexityI < complexityJ
		}

		// Tiebreaker: priority (lower number = higher priority)
		if bi.Priority != bj.Priority {
			return bi.Priority < bj.Priority
		}

		// Final tiebreaker: creation date (oldest first)
		return bi.CreatedAt.Before(bj.CreatedAt)
	})
}

// FetchEpicChildren fetches all ready children of an epic using bd ready --parent.
// Returns the children sorted by priority (ascending) then creation date (oldest first).
func FetchEpicChildren(runBD RunBDFunc, workDir string, epicID string, labels []string) ([]beads.Bead, error) {
	if runBD == nil {
		runBD = runBDReal
	}

	args := []string{"ready", "--json", "--parent", epicID}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	out, err := runBD(workDir, args...)
	if err != nil {
		return nil, fmt.Errorf("bd ready --parent %s: %w", epicID, err)
	}

	parsed, err := parseReadyBeads(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	// Sort by priority then creation date
	DefaultScorer(parsed)

	return parsed, nil
}

