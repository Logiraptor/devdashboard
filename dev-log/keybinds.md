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
- **Multi-level**: Supports `SPC p c`; after `SPC p`, stays in leader if `HasPrefix("SPC p")`
- **Dispatch order**: KeyHandler runs before views; consumed keys never reach views

## Default Bindings

| Sequence | Action | Context |
|----------|--------|---------|
| `q`, `ctrl+c` | Quit | Any |
| `SPC q` | Quit (spacemacs-style) | Any |
| `j`, `down` | Next item (navigates through beads within a resource before advancing) | Project detail |
| `k`, `up` | Previous item (navigates through beads within a resource before retreating) | Project detail |
| `g` | First item (resource header) | Project detail |
| `G` | Last item (last bead of last resource, or last resource header) | Project detail |
| `/` | Search/filter lines in resource view (vim-style) | Project detail |

## SPC p — Project Management

| Sequence | Action | Context |
|----------|--------|---------|
| `SPC p c` | Create project | Any (modal) |
| `SPC p d` | Delete selected project | Dashboard only |
| `SPC p a` | Add repo to project | Project detail |
| `SPC p r` | Remove repo from project | Project detail |
| `SPC p x` | Remove selected resource (kill panes, remove worktree) | Project detail |
| `d` | Remove selected resource (shortcut for SPC p x) | Project detail |

## Search Mode (`/` in Project Detail)

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

## SPC s — Shell / Agent

| Sequence | Action |
|----------|--------|
| `SPC s s` | Open shell (tmux pane in selected resource's worktree) |
| `SPC s a` | Launch agent (`agent` in selected resource's worktree) |
| `SPC s r` | Ralph loop — automated agent that picks work and implements it. When cursor is on a **bead**, sends targeted prompt for that specific bead ID; when on a **resource header**, sends generic `bd ready` prompt. Automatically injects `.cursor/rules/` and `dev-log/` into worktree (git-silent via `.git/info/exclude`) |
| `SPC s h` | Hide shell pane |
| `SPC s j` | Show shell pane |

## Help View

- Triggered when `KeyHandler.LeaderWaiting` is true (after SPC)
- `RenderKeybindHelp(reg)` produces transient help bar below content
- Format: `SPC  q: Quit  p: Projects  [esc] cancel`
- After `SPC p`, shows next-level hints: `c`, `d`, `a`, `r`
- No overlay stack; help is purely visual; KeyHandler consumes next key

## Tmux Keybinds (contrib/tmux.conf)

devdeploy uses SPC as leader; tmux uses **Ctrl+a** (C-a) to avoid accidental triggers in shells.

| Prefix | Submenu | Keys | Action |
|--------|---------|------|--------|
| C-a | **w** (windows) | w/W/n/c/0-9 | Next/prev/new/close/select window |
| C-a | **s** (splits) | s/v/x | Split H, split V, kill pane |
| C-a | **vim nav** | h/j/k/l | Focus pane left/down/up/right |

### Which-key (keybind discovery popup)

| Keys | Action |
|------|--------|
| **C-Space** | Open which-key menu (root, no prefix) |
| **C-a Space** | Open which-key menu (prefix + Space) |

- devdeploy (Bubble Tea) runs in one tmux pane; SPC remains its leader
- Both systems coexist; no conflict
