// Package rules provides embedded Cursor rule files for injection into worktrees.
//
// The canonical rule files (beads.mdc, devdeploy.mdc) are embedded at compile
// time and exposed via [Files] for worktree injection by the project manager.
package rules

import "embed"

//go:embed *.mdc
var ruleFS embed.FS

// Files returns the embedded rule files as a map of filename to content.
// The returned filenames are bare names (e.g. "beads.mdc", "devdeploy.mdc")
// suitable for writing into a worktree's .cursor/rules/ directory.
func Files() map[string][]byte {
	entries, err := ruleFS.ReadDir(".")
	if err != nil {
		// Should never happen — embedded FS is compiled in.
		return nil
	}
	out := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := ruleFS.ReadFile(e.Name())
		if err != nil {
			continue
		}
		out[e.Name()] = data
	}
	return out
}
