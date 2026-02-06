# Keybind Help View (Post-SPC)

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.4

## Context

Phase 4 required a transient help view triggered after SPC to improve discoverability. The epic calls for keybind hints that dismiss on next key or timeout.

## Decision

### KeybindRegistry Extensions

- **BindWithDesc(seq, cmd, desc)** — Register bindings with human-readable descriptions for the help view
- **Bind(seq, cmd)** — Unchanged; delegates to BindWithDesc with empty desc (Hints uses seq as fallback)
- **LeaderHints()** — Returns only SPC-prefixed bindings; keys are the suffix (e.g. `"q"`), values are descriptions

### Help View

- **RenderKeybindHelp(reg)** — Produces the transient help bar shown when `KeyHandler.LeaderWaiting` is true
- Displayed below the current view content when SPC is pressed
- Compact format: `SPC  q: Quit  [esc] cancel`
- Styled with lipgloss (rounded border, consistent colors with dashboard)

### Integration

- `appModelAdapter.View()` checks `KeyHandler.LeaderWaiting` and appends the help bar
- No overlay stack or input capture — help is purely visual; KeyHandler continues to consume the next key

## Implementation

- `internal/ui/keybind.go`: BindWithDesc, LeaderHints
- `internal/ui/keybind_help.go`: RenderKeybindHelp
- `internal/ui/app.go`: View() appends help when LeaderWaiting; NewAppModel uses BindWithDesc

## Consequences

- **Discoverability**: Users see available SPC commands without memorizing
- **Extensible**: New bindings added via BindWithDesc get hints automatically
- **Context-aware (future)**: LeaderHints could be filtered per mode/view in Phase 5+
