package ralph

import "time"

// bdShowBase contains the common fields shared across all bd show --json struct variants.
// This base type is embedded by specific variants that add their own fields.
type bdShowBase struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// bdShowReadyEntry is used for checking if a bead is ready to work on.
// It includes additional fields needed for readiness determination.
type bdShowReadyEntry struct {
	bdShowBase
	Priority        int       `json:"priority"`
	Labels          []string  `json:"labels"`
	CreatedAt       time.Time `json:"created_at"`
	IssueType       string    `json:"issue_type"`
	DependencyCount int       `json:"dependency_count"`
}
