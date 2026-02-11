// Package ralph implements the autonomous agent work loop for devdeploy.
//
// Ralph orchestrates AI agents to work through beads (issues) autonomously.
// It supports multiple execution modes:
//
//   - Sequential: Process beads one at a time (default)
//   - Concurrent: Process multiple beads in parallel using git worktrees
//   - Epic: Orchestrate all children of an epic sequentially with verification
//   - Wave: Dispatch all ready beads in parallel
//
// # Basic Usage
//
// The main entry point is the Run function:
//
//     cfg := ralph.LoopConfig{
//         WorkDir:       "/path/to/repo",
//         MaxIterations: 10,
//     }
//     summary, err := ralph.Run(ctx, cfg)
//
// # Safety Guards
//
// Ralph includes several safety mechanisms:
//   - Maximum iteration limit (default unlimited, configurable)
//   - Consecutive failure limit (default 3)
//   - Wall-clock timeout (default 2 hours)
//   - Same-bead retry detection
//
// # Dependency Injection
//
// LoopConfig supports test hooks for all external dependencies:
// PickNext, FetchPrompt, Render, Execute, AssessFn, SyncFn.
package ralph
