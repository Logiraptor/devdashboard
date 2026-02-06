# DevDeploy Architecture

**Status**: accepted  
**Last updated**: 2026-02-06

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

| Phase | Focus |
|-------|-------|
| 1 | Foundation: Bubble Tea, project structure, dev-log |
| 2 | UI abstractions: views, panels, focus, layout |
| 3 | Input: leader key (SPC), vim/spacemacs keybinds |
| 4 | Keybind hints: transient help after SPC |
| 5 | Agent workflow: artifact store, progress stream, integration |
| 6 | Live progress windows for agent output |
| 7 | Abort/cancel for in-flight operations |

**Dependencies**: Phase 1 → 2 → 3 → 4 (foundation); Phase 2 → 5 → 6 → 7 (agent stack)

## Open Questions

- Integration targets: Cursor, Claude Code, custom agents?
- Persistence: where do plans/context live beyond disk?
