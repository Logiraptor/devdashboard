# DevDeploy Architecture

**Status**: accepted  
**Last updated**: 2026-02-16

## Vision

DevDeploy aids engineers in their day-to-day workflow of designing, writing, reviewing, deploying, and testing code. Managing agent context, plans, and execution is a first-class concern.

## Core Requirements

- **Input**: vim + spacemacs keybinds, SPC leader, keybind hints after SPC
- **Agent UX**: live progress windows, abort at any time
- **Tech**: Go + Bubble Tea / Charm Bracelet, invest in right abstractions for complex TUI

## Technology Stack

| Component | Choice |
|-----------|--------|
| TUI framework | Bubble Tea (Elm Architecture) |
| Styling | Lipgloss |
| Components | Bubbles (spinner, viewport, etc.) |

## Project Structure

```
devdeploy/
├── cmd/devdeploy/     # Main TUI entrypoint
├── cmd/ralph/         # Standalone headless CLI for autonomous agent loops
├── internal/          # Private packages
│   ├── trace/         # OTLP trace export for ralph CLI
│   └── ralph/         # Core execution engine and TUI
├── dev-log/           # Architecture decision records
├── contrib/           # Tmux config, etc.
├── go.mod
└── go.sum
```

## UI Modeling Conventions

- **Model = state** — Each screen/view has a model struct
- **Update = message handler** — Use `tea.Msg` for all events
- **View = pure render** — No side effects; derive from model only
- **Composition** — Nest models for complex layouts

## Phased Roadmap

| Phase | Focus | Status |
|-------|-------|--------|
| 1 | Foundation: Bubble Tea, project structure, dev-log | Done |
| 2 | UI abstractions: views, panels, focus, layout | Done |
| 3 | Input: leader key (SPC), vim/spacemacs keybinds | Done |
| 4 | Keybind hints: transient help after SPC | Done |
| 5 | Agent workflow: progress stream, integration | Done |
| 6 | Live progress windows for agent output | Done |
| 7 | Abort/cancel for in-flight operations | Done |
| 8 | Tmux pane orchestration (replace embedded PTY) | Done |
| 9 | Resource-based project workflow | Done |
| 10 | Automated agent loop (Ralph) | Done |

**Phase 9 — Resource-based project workflow**: Projects contain resources (repos from ~/workspace, PRs from gh). Two actions: open shell, launch agent (`agent`). devdeploy manages worktrees, tmux panes, and sessions. Resources display associated **beads** (bd issues) inline via label-based scoping. See `devdeploy-7uj` and `devdeploy-lvr` epics.

**Phase 10 — Automated agent loop (Ralph)**: Ralph loop (`SPC s r`) launches an agent with a canned prompt to pick work and implement it. Cursor rules (beads.mdc, devdeploy.mdc) are managed externally via `~/workspace/cursor-config`. See `devdeploy-j4n` epic.

### cmd/ralph — Standalone Headless CLI

`cmd/ralph` is a standalone Go CLI binary for autonomous agent work loops. It can be run independently of the devdeploy TUI, making it suitable for headless execution, CI/CD pipelines, or background processing.

**Usage**:
```bash
ralph --workdir=<path> --bead=<id> [flags]
```

**Key features**:
- **Parallel execution**: Processes multiple beads concurrently (configurable via `--max-parallel`, default 4)
- **TUI mode**: Interactive display showing multiple agent blocks with live tool event streaming (see [ui.md](ui.md#multi-agent-parallel-tui))
- **OTLP tracing**: Exports traces via OpenTelemetry Protocol for observability (see `internal/trace` package)
- **Timeout handling**: Per-agent execution timeout (default 10 minutes, configurable via `--agent-timeout`)
- **Graceful shutdown**: Handles SIGINT for clean termination

**Exit codes**:
- `0`: Normal completion (all beads processed)
- `1`: Runtime error
- `5`: Interrupted (SIGINT)

The CLI uses the same `ralph.Core` execution engine as the TUI integration, ensuring consistent behavior. See `dev-log/agent-workflow.md` for details on the ProgressObserver interface and tool event streaming.

### internal/trace — OTLP Trace Export

The `internal/trace` package provides OpenTelemetry Protocol (OTLP) trace export for ralph execution. It's used by `cmd/ralph` to export structured traces for observability.

**Components**:
- **`Manager`**: Manages trace spans and exports completed traces via OTLP
- **`OTLPExporter`**: Handles OTLP HTTP/gRPC export to configured endpoint
- **`TracingObserver`**: Implements `ralph.ProgressObserver` to capture loop/bead/tool events as trace spans

**Configuration**:
- Set `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable (e.g., `"http://localhost:4318"`)
- If not configured, tracing operates as a no-op (no export occurs)

**Trace structure**:
- **Loop span**: Root span for entire execution loop
- **Iteration spans**: Child spans for each bead execution
- **Tool spans**: Child spans for individual tool calls, parented to iteration spans

See `internal/ralph/trace_observer.go` for implementation details.

## Core Concept

devdeploy is a glue tool for **git worktrees**, **agent sessions**, **GitHub PRs**, **tmux panes**, and **beads** (bd issue tracker). Projects group resources; the primary actions are opening a shell or launching an agent in a worktree. Beads are displayed per resource via label-based scoping (`project:` and `pr:` labels). Everything persists until explicitly cleaned up.

## Open Questions

- Future: could add keybinds to open/close beads from within devdeploy
