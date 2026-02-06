package artifact

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProjectDirEnv is the env var override for ~/.devdeploy base (for testing).
	ProjectDirEnv = "DEVDEPLOY_PROJECTS_DIR"
	// DefaultProjectsBase is the default base for project directories.
	DefaultProjectsBase = ".devdeploy/projects"
)

// Store reads plan and design artifacts from project directories.
// Layout: ~/.devdeploy/projects/<name>/plan.md, design.md
type Store struct {
	baseDir string
}

// ProjectArtifacts holds plan and design content for a project.
type ProjectArtifacts struct {
	Plan   string
	Design string
}

// NewStore creates a store rooted at the user's home + DefaultProjectsBase,
// or at the path in DEVDEPLOY_PROJECTS_DIR if set.
func NewStore() (*Store, error) {
	base := os.Getenv(ProjectDirEnv)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, DefaultProjectsBase)
	}
	return &Store{baseDir: base}, nil
}

// ProjectDir returns the absolute path for a project by name.
func (s *Store) ProjectDir(name string) string {
	// Normalize: lowercase, replace spaces with hyphens
	normalized := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	return filepath.Join(s.baseDir, normalized)
}

// Load reads plan.md and design.md from the project directory.
// Missing files yield empty strings; no error for missing dir or files.
func (s *Store) Load(projectName string) ProjectArtifacts {
	dir := s.ProjectDir(projectName)
	return ProjectArtifacts{
		Plan:   readFile(filepath.Join(dir, "plan.md")),
		Design: readFile(filepath.Join(dir, "design.md")),
	}
}

func readFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// ArtifactSummary returns a short label for display (e.g. "Plan (excerpt)" or "no plan yet").
func (a ProjectArtifacts) PlanSummary() string {
	if a.Plan == "" {
		return "no plan yet"
	}
	lines := strings.SplitN(a.Plan, "\n", 2)
	first := strings.TrimSpace(lines[0])
	if len(first) > 60 {
		first = first[:57] + "..."
	}
	return first
}

func (a ProjectArtifacts) DesignSummary() string {
	if a.Design == "" {
		return "no design yet"
	}
	lines := strings.SplitN(a.Design, "\n", 2)
	first := strings.TrimSpace(lines[0])
	if len(first) > 60 {
		first = first[:57] + "..."
	}
	return first
}
