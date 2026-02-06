package ui

// AppMode represents the top-level application mode (Option E: Dashboard + Detail).
type AppMode int

const (
	ModeDashboard AppMode = iota
	ModeProjectDetail
)

func (m AppMode) String() string {
	switch m {
	case ModeDashboard:
		return "Dashboard"
	case ModeProjectDetail:
		return "ProjectDetail"
	default:
		return "Unknown"
	}
}
