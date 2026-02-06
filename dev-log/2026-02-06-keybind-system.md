# Keybind System: Leader Key and vim/spacemacs Keybinds

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.3

## Context

Phase 3 required implementing leader key (SPC) handling, vim/spacemacs-style keybind parsing, and a keybind registry with command mapping. The epic calls for familiar modal editing and navigation with discoverability via SPC.

## Decision

### KeybindRegistry

- Maps key sequences to `tea.Cmd` (Bubble Tea commands)
- Key sequences use spacemacs-style notation: `"SPC"` for space, `"SPC f"` for SPC then f
- Single keys: `"j"`, `"k"`, `"esc"`, `"ctrl+c"`, `"enter"`
- `Bind(seq, cmd)` and `Lookup(seq)` for registration and dispatch
- `Hints()` returns all bound sequences (for Phase 4 help view)

### KeyHandler

- **Leader key**: Space (`" "` — Bubble Tea reports KeySpace as literal space)
- **Leader mode**: After SPC, waits for next key; builds sequence like `"SPC x"`
- **Esc cancels**: Leader mode is cancelled by Esc without executing
- **Dispatch order**: KeyHandler runs before views; consumed keys never reach views

### Default Bindings

| Sequence | Action |
|----------|--------|
| `q` | Quit |
| `ctrl+c` | Quit |
| `SPC q` | Quit (spacemacs-style) |

### vim-style Navigation (Dashboard)

| Key | Action |
|-----|--------|
| `j`, `down` | Next item |
| `k`, `up` | Previous item |
| `g` | First item |
| `G` | Last item |

## Implementation

- `internal/ui/keybind.go`: KeybindRegistry, KeyHandler, sequence normalization
- `internal/ui/app.go`: KeyHandler integrated into Update; default bindings in NewAppModel
- Views receive only keys not consumed by KeyHandler

## Consequences

- **Phase 4 ready**: Hints() provides data for keybind help view
- **Extensible**: New bindings added via `reg.Bind()`; context-aware bindings possible per mode
- **Bubble Tea quirk**: Space is `" "` not `"space"` in KeyMsg.String()
