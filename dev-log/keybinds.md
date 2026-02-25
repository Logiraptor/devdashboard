# Keybind System

**Status**: accepted  
**Last updated**: 2026-02-09

## KeybindRegistry

- Maps key sequences to `tea.Cmd`
- Spacemacs-style notation: `"SPC"` for space, `"SPC f"` for SPC then f
- Single keys: `"j"`, `"k"`, `"esc"`, `"ctrl+c"`, `"enter"`
- `Bind(seq, cmd)` / `BindWithDesc(seq, cmd, desc)` for registration
- `Lookup(seq)` for dispatch; `LeaderHints(prefix)` for help view
- **Bubble Tea quirk**: Space is `" "` not `"space"` in `KeyMsg.String()`

## KeyHandler

- **Leader key**: Space (`" "`)
- **Leader mode**: After SPC, waits for next key; builds sequence like `"SPC x"`
- **Esc cancels**: Leader mode cancelled by Esc without executing
- **Multi-level**: Supports sequences like `SPC p a`; after `SPC p`, stays in leader if `HasPrefix("SPC p")`
- **Dispatch order**: KeyHandler runs before views; consumed keys never reach views

## Default Bindings

| Sequence | Action | Context |
|----------|--------|---------|
| `q`, `ctrl+c` | Quit | Any |
| `SPC q` | Quit (spacemacs-style) | Any |
| `j`, `down` | Next item (navigates through beads within a resource before advancing) | Home |
| `k`, `up` | Previous item (navigates through beads within a resource before retreating) | Home |
| `g` | First item (resource header) | Home |
| `G` | Last item (last bead of last resource, or last resource header) | Home |
| `/` | Search/filter lines in resource view (vim-style) | Home |

## SPC p — Resource Management

| Sequence | Action | Context |
|----------|--------|---------|
| `SPC p a` | Add worktree (prompts for branch name) | Home (repo selected) |
| `SPC p x` | Remove selected resource (kill session, remove worktree) | Home |
| `d` | Remove selected resource (shortcut for SPC p x) | Home |

## Search Mode (`/` in Home)

Pressing `/` activates vim-style search mode for filtering and jumping to lines in the resource view.

| Key | Action |
|-----|--------|
| `/` | Activate search mode (shows search prompt) |
| `Enter` | Accept search and jump to first match (exits input mode, stays in search for n/N) |
| `n` | Next match (when search is active, input not focused) |
| `N` | Previous match (when search is active, input not focused) |
| `Esc` | Cancel search (exits search mode entirely) |

**Search behavior:**
- Search is case-insensitive and matches any text in resource names, bead IDs, and bead titles
- While typing the search query, matches update in real-time
- After pressing Enter, use `n`/`N` to navigate between matches
- Press `/` again while in search navigation mode to start a new search
- Search prompt shows match count: `[current/total]` or `[no matches]`

## SPC s — Session Actions

| Sequence | Action |
|----------|--------|
| `enter` | Open/switch to selected resource session |
| `SPC s k` | Kill selected resource session |
| `SPC s c` | Open Cursor IDE on selected resource's worktree |

## SPC r — Refresh Beads

| Sequence | Action | Context |
|----------|--------|---------|
| `SPC r` | Refresh beads for all resources | Home only |

In home view, `SPC r` reloads beads for all resources without reloading repos or PRs. Useful when beads are updated externally (e.g., via CLI `bd close`).

## SPC b — Bead Operations

| Sequence | Action | Context |
|----------|--------|---------|
| `SPC b r` | Refresh beads for all resources | Home only |
| `SPC b c` | Close selected bead (marks as closed via bd close) | Home only |

Bead-related operations. `SPC b r` is an alias for `SPC r` (refresh beads).

## Help View

- Triggered when `KeyHandler.LeaderWaiting` is true (after SPC)
- `RenderKeybindHelp(reg)` produces transient help bar below content
- Format: `SPC  q: Quit  p: Resources  [esc] cancel`
- After `SPC p`, shows next-level hints: `a`, `x`
- No overlay stack; help is purely visual; KeyHandler consumes next key

## Tmux Keybinds (contrib/tmux.conf)

Simple vim-style pane navigation (no prefix required):
- **C-h**: Focus pane left
- **C-j**: Focus pane down
- **C-k**: Focus pane up
- **C-l**: Focus pane right

Prefix (C-a) operations:
- **C-a w/W**: Next/prev window
- **C-a c**: New window
- **C-a x**: Kill pane
- **C-a s/v**: Split horizontal/vertical
- **C-a |/-**: Split horizontal/vertical (alternative bindings)

- devdeploy uses SPC as leader; tmux uses **Ctrl+a** (C-a) to avoid accidental triggers in shells
- devdeploy (Bubble Tea) runs in one tmux pane; SPC remains its leader
- Both systems coexist; no conflict
