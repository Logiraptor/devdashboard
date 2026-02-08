# UI Abstraction and Layout

**Status**: accepted  
**Last updated**: 2026-02-06

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

**Rationale**: Balances visibility and simplicity. Matches project-centric workflow (2–3 projects, many PRs). Artifacts integrated per project. Scales to many projects.

- **Dashboard**: Lists all projects with summary. Select one to open detail.
- **Project Detail**: Splits for repos/PRs, dedicated artifact area (plan, design doc).
- **Overlays**: Modals for create/delete project, repo picker, progress window.

**Model shape**:
```go
type AppModel struct {
    Mode       AppMode           // Dashboard | ProjectDetail
    Dashboard  DashboardModel    // List of projects + summaries
    Detail     ProjectDetailModel // Selected project: splits + artifacts
    Artifacts  ArtifactStore    // Plan, design doc per project
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
- **Config + artifacts co-located**: `config.yaml`, `plan.md`, `design.md` in project root

```
~/.devdeploy/projects/
  ha-sampler-querier/
    config.yaml       # project config
    plan.md           # agent plan
    design.md         # design doc
    repo-a/           # worktree for first repo
    repo-b/           # worktree for second repo
```

- `DEVDEPLOY_PROJECTS_DIR` env overrides base path
- Project names normalized: lowercase, spaces → hyphens

## Artifacts

- Plain files on disk; editable with any tool
- "Include in every session" = agents link to file path; no special metadata
- Missing files → empty or "no plan yet" state
