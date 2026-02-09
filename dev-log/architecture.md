# DevDeploy Architecture

**Status**: accepted  
**Last updated**: 2026-02-08

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
├── cmd/devdeploy/     # Entrypoint
├── internal/          # Private packages
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
| 10 | Automated agent loop (Ralph) with git-silent rule injection | Done |

**Phase 9 — Resource-based project workflow**: Projects contain resources (repos from ~/workspace, PRs from gh). Two actions: open shell, launch agent (`agent`). devdeploy manages worktrees, tmux panes, and sessions. Resources display associated **beads** (bd issues) inline via label-based scoping. See `devdeploy-7uj` and `devdeploy-lvr` epics.

**Phase 10 — Automated agent loop (Ralph)**: Ralph loop (`SPC s r`) launches an agent with a canned prompt to pick work and implement it. Worktrees automatically receive `.cursor/rules/` (beads.mdc, devdeploy.mdc) and `dev-log/` directory injected via `.git/info/exclude` (git-silent, never committed). Rule injection is idempotent and happens automatically when worktrees are created or ensured. See `devdeploy-j4n` epic.

## Core Concept

devdeploy is a glue tool for **git worktrees**, **agent sessions**, **GitHub PRs**, **tmux panes**, and **beads** (bd issue tracker). Projects group resources; the primary actions are opening a shell or launching an agent in a worktree. Beads are displayed per resource via label-based scoping (`project:` and `pr:` labels). Everything persists until explicitly cleaned up.

## Open Questions

- Future: could add keybinds to open/close beads from within devdeploy
