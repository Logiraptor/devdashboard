# Agent Progress and Abort — Validation Checklist

**Date**: 2026-02-06  
**Related**: dev-log/2026-02-06-phase6-live-progress-windows.md, dev-log/2026-02-06-phase7-abort-capability.md, beads devdeploy-i1u.10

## Prerequisites

- Run `go run ./cmd/devdeploy` in a terminal with TTY
- Ensure `~/.devdeploy/projects/` exists (or set `DEVDEPLOY_PROJECTS_DIR`)
- Create at least one project and select it (enter Project detail mode)

## Checklist

### Agent progress visible

| Step | Action | Expected |
|------|--------|----------|
| 1 | `SPC a a` in Project detail | ProgressWindow overlay appears with "Agent progress" header |
| 2 | Wait for stub output | Events stream: "Agent run started (stub)", "Loading plan...", etc. |
| 3 | View updates | Timestamps, status icons (● running, ✓ done), messages visible |

### Abort stops execution

| Step | Action | Expected |
|------|--------|----------|
| 1 | `SPC a a` in Project detail | ProgressWindow appears |
| 2 | Press Esc during run | Run aborts; "Aborted" with ⊗ icon appears |
| 3 | Press Esc again | Overlay dismisses |

## Automated tests

- `go test ./internal/ui/... -run TestAgentProgressVisible` — ProgressWindow displays progress events
- `go test ./internal/ui/... -run TestAgentAbort` — Esc triggers cancel on in-flight run
- `go test ./internal/agent/... -run TestEmitAfter` — emitAfter respects context cancellation
