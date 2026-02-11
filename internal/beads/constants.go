package beads

// Status constants for bead status field.
const (
	StatusOpen       = "open"
	StatusInProgress = "in_progress"
	StatusClosed     = "closed"
)

// Label constants for common labels.
const (
	LabelNeedsHuman = "needs-human"
	LabelPRPrefix   = "pr:"
)

// DependencyType constants.
const (
	DepTypeParentChild = "parent-child"
	DepTypeBlocks      = "blocks"
)
