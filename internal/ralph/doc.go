// Package ralph implements the autonomous agent work loop for devdeploy.
//
// Ralph orchestrates AI agents to work through beads (issues) autonomously.
// It processes beads in parallel using git worktrees for isolation.
//
// # Basic Usage
//
// The main entry point is Core.Run:
//
//	core := &ralph.Core{
//	    WorkDir:     "/path/to/repo",
//	    RootBead:    "my-epic",    // optional: filter to epic's children
//	    MaxParallel: 4,            // concurrent agents
//	}
//	result, err := core.Run(ctx)
//
// # Progress Observation
//
// Implement ProgressObserver to receive live updates:
//
//	core.Observer = myObserver  // receives OnBeadStart, OnBeadComplete, etc.
//
// # Testing
//
// Core supports test hooks for all external dependencies:
// RunBD, FetchPrompt, Render, Execute, AssessFn.
package ralph
