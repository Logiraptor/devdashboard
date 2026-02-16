package beads

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"devdeploy/internal/bd"
)

// Bead represents a bd issue associated with a project resource.
type Bead struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Priority    int       `json:"priority"`
	Labels      []string  `json:"labels"`
	CreatedAt   time.Time `json:"created_at"`
	IssueType   string    `json:"issue_type"` // "epic", "task", "bug", etc.
	ParentID    string    `json:"parent_id"`  // parent epic ID (from parent-child dependency)
}

// bdDependency mirrors a single dependency entry in bd list JSON output.
type bdDependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// bdListEntry mirrors the JSON shape emitted by `bd list --json`.
type bdListEntry struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	Priority     int            `json:"priority"`
	Labels       []string       `json:"labels"`
	CreatedAt    time.Time      `json:"created_at"`
	IssueType    string         `json:"issue_type"`
	Dependencies []bdDependency `json:"dependencies"`
}

// runBD is the function used to execute bd commands.
// Replaced in tests for deterministic output.
var runBD = bd.Run

// ListForRepo runs `bd list --json` in the given worktree directory.
// Returns all beads in the repo, excluding any with a pr:<n> label
// (those belong to PR resources).
func ListForRepo(worktreeDir, projectName string) []Bead {
	out, err := runBD(worktreeDir,
		"list",
		"--json",
		"--limit", "0",
	)
	if err != nil {
		log.Printf("beads.ListForRepo: failed to run bd list in %q: %v", worktreeDir, err)
		return nil
	}
	all := parseBeads(out)

	// Exclude beads that have any pr:* label — those belong to PR resources.
	result := make([]Bead, 0, len(all))
	for _, b := range all {
		if !hasPRLabel(b.Labels) {
			result = append(result, b)
		}
	}
	return SortHierarchically(result)
}

// ListForPR runs `bd list --label pr:<number> --json` in the given worktree
// directory. Returns beads associated with this specific PR.
func ListForPR(worktreeDir, projectName string, prNumber int) []Bead {
	out, err := runBD(worktreeDir,
		"list",
		"--label", fmt.Sprintf("pr:%d", prNumber),
		"--json",
		"--limit", "0",
	)
	if err != nil {
		log.Printf("beads.ListForPR: failed to run bd list for pr:%d in %q: %v", prNumber, worktreeDir, err)
		return nil
	}
	return SortHierarchically(parseBeads(out))
}

// parseBeads decodes JSON output from bd list into Bead slice.
// Filters to open/in_progress by default (closed beads are noise).
func parseBeads(data []byte) []Bead {
	var entries []bdListEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("beads.parseBeads: failed to unmarshal JSON: %v", err)
		return nil
	}

	result := make([]Bead, 0, len(entries))
	for _, e := range entries {
		if e.Status == StatusClosed {
			continue
		}
		result = append(result, Bead{
			ID:          e.ID,
			Title:       e.Title,
			Description: e.Description,
			Status:      e.Status,
			Priority:    e.Priority,
			Labels:      e.Labels,
			CreatedAt:   e.CreatedAt,
			IssueType:   e.IssueType,
			ParentID:    extractParentID(e.Dependencies),
		})
	}
	return result
}

// extractParentID returns the parent epic ID from a parent-child dependency,
// or "" if none exists.
func extractParentID(deps []bdDependency) string {
	for _, d := range deps {
		if d.Type == DepTypeParentChild {
			return d.DependsOnID
		}
	}
	return ""
}

// SortHierarchically reorders beads so that epics appear first, each
// immediately followed by their children, then standalone beads (no parent,
// not an epic). Within each group, original order is preserved.
func SortHierarchically(beads []Bead) []Bead {
	if len(beads) <= 1 {
		return beads
	}

	// Index beads by ID for lookup.
	byID := make(map[string]*Bead, len(beads))
	for i := range beads {
		byID[beads[i].ID] = &beads[i]
	}

	// Group children by parent ID, preserving order.
	childrenOf := make(map[string][]Bead)
	var epics []Bead
	var standalone []Bead

	for _, b := range beads {
		switch {
		case b.IssueType == "epic" && b.ParentID == "":
			epics = append(epics, b)
		case b.ParentID != "":
			childrenOf[b.ParentID] = append(childrenOf[b.ParentID], b)
		default:
			standalone = append(standalone, b)
		}
	}

	// Sort epics by priority (lower = higher priority), then by ID for stability.
	sort.SliceStable(epics, func(i, j int) bool {
		if epics[i].Priority != epics[j].Priority {
			return epics[i].Priority < epics[j].Priority
		}
		return epics[i].ID < epics[j].ID
	})

	result := make([]Bead, 0, len(beads))

	// Emit each epic followed by its children.
	for _, epic := range epics {
		result = append(result, epic)
		children := childrenOf[epic.ID]
		// Sort children by priority then ID.
		sort.SliceStable(children, func(i, j int) bool {
			if children[i].Priority != children[j].Priority {
				return children[i].Priority < children[j].Priority
			}
			return children[i].ID < children[j].ID
		})
		result = append(result, children...)
		delete(childrenOf, epic.ID)
	}

	// Any remaining children whose parent epic wasn't in this list
	// (parent might be closed or in a different resource) — treat as standalone.
	for _, orphans := range childrenOf {
		standalone = append(standalone, orphans...)
	}

	// Sort standalone by priority then ID.
	sort.SliceStable(standalone, func(i, j int) bool {
		if standalone[i].Priority != standalone[j].Priority {
			return standalone[i].Priority < standalone[j].Priority
		}
		return standalone[i].ID < standalone[j].ID
	})

	result = append(result, standalone...)
	return result
}

// hasPRLabel reports whether labels contains a "pr:<number>" label.
func hasPRLabel(labels []string) bool {
	for _, l := range labels {
		if strings.HasPrefix(l, LabelPRPrefix) {
			return true
		}
	}
	return false
}
