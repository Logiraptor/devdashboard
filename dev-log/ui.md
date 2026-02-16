# UI Abstraction and Layout

**Status**: accepted  
**Last updated**: 2026-02-16

## Core Abstractions

| Abstraction | Purpose |
|-------------|---------|
| **View** | Screen or major UI region with model, update, view |
| **AppModel** | Root model; switches between Dashboard and ProjectDetail modes |
| **OverlayStack** | Modal/popup views layered on top of the active mode |
| **KeyHandler** | Leader-key (SPC) keybind system with mode-aware bindings |

> **Note**: Earlier designs included Panel, Layout, FocusManager, and ViewStack abstractions.
> These were removed (2026-02-07) as unused — the app uses AppModel mode switching + OverlayStack instead.

## Chosen Layout: Dashboard + Detail (Option E)

**Rationale**: Balances visibility and simplicity. Matches project-centric workflow (2–3 projects, many PRs). Scales to many projects.

- **Dashboard**: Lists all projects with summary. Select one to open detail.
- **Project Detail**: Shows repos/PRs as a unified resource list.
- **Overlays**: Modals for create/delete project, repo picker, progress window.

**Model shape**:
```go
type AppModel struct {
    Mode       AppMode           // Dashboard | ProjectDetail
    Dashboard  DashboardModel    // List of projects + summaries
    Detail     ProjectDetailModel // Selected project: resources
    Overlays   []Overlay        // Modal stack
    Focus      FocusTarget
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

## Project Directory Layout

- **One directory per project**: `~/.devdeploy/projects/<project-name>/`
- **Worktrees inside**: each repo gets a worktree subdir (`repo-a/`, `repo-b/`)
- **Config co-located**: `config.yaml` in project root

```
~/.devdeploy/projects/
  ha-sampler-querier/
    config.yaml       # project config
    repo-a/           # worktree for first repo
    repo-b/           # worktree for second repo
```

- `DEVDEPLOY_PROJECTS_DIR` env overrides base path
- Project names normalized: lowercase, spaces → hyphens

## Beads per Resource

Each resource (repo or PR) displays associated **beads** (bd issues) inline in the project detail view.

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

**Project detail view** — beads listed under each resource:

```
← my-project

Resources
▸ devdeploy/              ● 2 shells
    devdeploy-abc  Fix the thing
    devdeploy-def  Add feature X  [in_progress]
  #42 Add dark mode (open)   ● 1 agent
    devdeploy-ghi  Review PR feedback
```

**Dashboard** — bead count shown per project alongside repo/PR counts.

### Bead Details Section

The project detail view includes a **Bead Details** section at the bottom that displays additional information about the currently selected bead.

**Behavior**:
- Shows when the cursor is positioned on a bead item (not a resource header)
- Displays placeholder text `(select a bead to see details)` when no bead is selected
- Fixed height (7 lines) to prevent layout jumping when selection changes

**Content displayed**:
- **Title**: Bead ID and title (e.g., `devdeploy-abc  Fix the thing`)
- **Status**: Status and issue type (e.g., `in_progress  task`)
- **Description**: Full description text, truncated to fit width (max 2 lines)
- **Labels**: Comma-separated list of labels, truncated if too long

**Implementation**:
- `BeadInfo` struct (`internal/project/resource.go`) includes `Description` and `Labels` fields
- Rendered by `renderBeadDetailsSection()` in `internal/ui/projectdetail.go`
- Uses `SelectedBead()` to determine which bead to display

### History

The artifact system (plan.md / design.md) was removed in 2026-02-08 (see `devdeploy-lvr` epic). Beads integration replaced it as the primary way to track work items per resource.
