# TUI Foundation and UI Modeling Approach

**Date**: 2026-02-06
**Status**: accepted

## Context

Phase 1 of DevDeploy requires initializing a Go module with Bubble Tea, Lipgloss, and Bubbles. The epic calls for investing in the right abstractions to make modeling complex UIs easier from the start. Before Phase 2 designs the abstraction layer, we need to document the architectural assumptions and conventions.

## Decision

### Technology Stack
- **Bubble Tea** — Elm Architecture for TUI state management (model, update, view)
- **Lipgloss** — Declarative styling (colors, borders, layout)
- **Bubbles** — Reusable components (spinner, viewport, etc.)

### Project Structure
```
devdeploy/
├── cmd/devdeploy/     # Entrypoint
├── internal/          # Private packages (to add in Phase 2)
├── dev-log/           # Architecture decision records
├── go.mod
└── go.sum
```

### UI Modeling Conventions (for Phase 2)
- **Model = state** — Each screen/view has a model struct
- **Update = message handler** — Use tea.Msg for all events
- **View = pure render** — No side effects; derive from model only
- **Composition** — Nest models for complex layouts (parent delegates to child models)

### Dev-Log Conventions
- All major architecture decisions go in `dev-log/`
- Format: `YYYY-MM-DD-topic-name.md`
- Status: proposed → accepted → deprecated

## Consequences

- Bubble Tea's Elm Architecture is well-suited for predictable state updates
- Lipgloss v1 is stable; v2 exists but we start with v1 for compatibility
- Phase 2 will design the abstraction layer (views, panels, focus) on top of this foundation
- `internal/` reserved for packages that should not be imported externally
