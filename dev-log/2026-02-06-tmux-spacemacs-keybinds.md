# Tmux Spacemacs-Style Keybinds (Non-SPC Leader)

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-e56

## Context

devdeploy uses SPC (space) as its leader key for Spacemacs-style keybinds (SPC p, SPC a, etc.). Tmux runs in a terminal where space is typed constantly — using SPC as tmux's prefix would trigger accidental commands. We need a tmux config that mirrors the Spacemacs-style structure (leader + submenus) but with a different leader.

## Decision

### Leader Key

Use **Ctrl+a** (C-a) as the tmux prefix. Rationale:

- Space is unsuitable (typed constantly in shells)
- C-a is familiar from screen; easy to reach
- Alternative: C-e (E for devdeploy) — documented as option in config

### Spacemacs-Like Submenus

| Prefix | Submenu | Keys | Action |
|--------|---------|------|--------|
| C-a | **w** (windows) | w | Next window |
| | | W | Previous window |
| | | n | New window |
| | | c | Close window |
| | | 0-9 | Select window by index |
| C-a | **s** (splits) | s | Split horizontal |
| | | v | Split vertical |
| | | x | Kill pane |
| C-a | **vim nav** | h | Focus pane left |
| | | j | Focus pane down |
| | | k | Focus pane up |
| | | l | Focus pane right |

### Relation to devdeploy

- devdeploy (Bubble Tea TUI) runs in one tmux pane; SPC remains its leader
- Tmux keybinds operate at the multiplexer level; no conflict
- Users run devdeploy inside tmux; both systems coexist

## Implementation

- `contrib/tmux.conf` — sourceable tmux config with the above bindings
- Users: `tmux source-file /path/to/devdeploy/contrib/tmux.conf` or add to `~/.tmux.conf`:
  ```bash
  source-file ~/path/to/devdeploy/contrib/tmux.conf
  ```

## Consequences

- Consistent mental model: leader + submenu (w/s) across devdeploy and tmux
- Vim-style pane navigation (hjkl) for muscle memory
- No accidental triggers from spacebar in shells
