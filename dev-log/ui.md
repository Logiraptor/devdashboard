# UI Abstraction and Layout

**Status**: accepted  
**Last updated**: 2026-02-25

## Core Abstractions

| Abstraction | Purpose |
|-------------|---------|
| **View** | Screen or major UI region with model, update, view |
| **AppModel** | Root model for the repo/resource home view |
| **OverlayStack** | Modal/popup views layered on top of the active mode |
| **KeyHandler** | Leader-key (SPC) keybind system with mode-aware bindings |

> **Note**: Earlier designs included Panel, Layout, FocusManager, and ViewStack abstractions.
> These were removed (2026-02-07) as unused — the app uses AppModel mode switching + OverlayStack instead.

## Chosen Layout: Home + Resources

**Rationale**: Keep a single repo-centric surface where each repo, worktree, and PR is a resource. This matches the v2 model and removes project switching overhead.

- **Home**: Lists workspace repos and their resources in one view.
- **Resources**: Repos, worktrees, and PRs rendered as a unified list with nested beads.
- **Overlays**: Modals for add/remove resource operations and progress windows.

**Model shape**:
```go
type AppModel struct {
    Home      HomeModel
    Overlays  []Overlay
    Focus     FocusTarget
}
```

## Implementation Interfaces

```go
type View interface {
    Init() tea.Cmd
    Update(tea.Msg) (View, tea.Cmd)
    View() string
}

type Overlay struct {
    View   View
    Dismiss tea.Key  // e.g. Esc
}
```

## Workspace Resource Layout

- **Repo roots** live in `~/workspace/<repo>`
- **Worktrees** live as sibling paths under workspace (for example `~/workspace/<repo>--<branch>`)
- **PR resources** resolve to matching worktree paths when present

## Beads per Resource

Each resource (repo, worktree, or PR) displays associated **beads** (bd issues) inline in the home view.

### Scoping

- **Repo resource**: All beads in the repo (excluding PR-labeled ones)
- **PR resource**: Beads with `pr:<number>` label

### Query logic

| Resource | Command | Filter |
|----------|---------|--------|
| Repo | `bd list --json` in repo worktree | Exclude beads with any `pr:*` label |
| PR | `bd list --label pr:<number> --json` in repo worktree | None |

Closed beads are filtered out (only open/in_progress shown).

### Display

**Home view** — beads listed under each resource:

```
Resources
▸ devdeploy/              ● 2 shells
    devdeploy-abc  Fix the thing
    devdeploy-def  Add feature X  [in_progress]
  #42 Add dark mode (open)   ● 1 agent
    devdeploy-ghi  Review PR feedback
```

Bead counts are shown directly on resources in the home list.

### Bead Details Section

The home view includes a **Bead Details** section at the bottom that displays additional information about the currently selected bead.

**Behavior**:
- Shows when the cursor is positioned on a bead item (not a resource header)
- Displays placeholder text `(select a bead to see details)` when no bead is selected
- Dynamic height (5-15 lines) based on terminal size, consistent within session to prevent layout jumping

**Content displayed**:
- **Title**: Bead ID and title (e.g., `devdeploy-abc  Fix the thing`)
- **Status**: Status and issue type (e.g., `in_progress  task`)
- **Description**: Full description text, shows as much as will fit (calculated from section height)
- **Labels**: Comma-separated list of labels, truncated if too long

**Implementation**:
- `BeadInfo` struct (`internal/project/resource.go`) includes `Description` and `Labels` fields
- Rendered by `renderBeadDetailsSection()` in `internal/ui/home.go`
- Uses `SelectedBead()` to determine which bead to display

### History

The artifact system (plan.md / design.md) was removed in 2026-02-08 (see `devdeploy-lvr` epic). Beads integration replaced it as the primary way to track work items per resource.

## Removed Components

The Ralph multi-agent TUI and related `internal/ralph` views were removed in V2.  
This document intentionally no longer describes `MultiAgentView`/`AgentBlock` behavior since those components are no longer part of devdeploy.
