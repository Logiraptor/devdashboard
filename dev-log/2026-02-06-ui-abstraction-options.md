# UI Abstraction Options for Complex TUI Modeling

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.2

## Context

Phase 2 requires designing an abstraction layer for complex TUI composition. The epic calls for modeling views, panels, focus management, and layout—with consideration for view stack, modal overlays, and split layouts.

**Workflow assumptions** (from user context):

- **Projects** — span 2–3 repos and 10–15 PRs each
- **Active projects** — typically 2–3 at a time
- **Visibility** — all of the above in one tool
- **Long-running artifacts** — agent plans, design docs that persist within a project and are included in almost every agent session

The abstractions must support this project-centric, artifact-aware workflow while fitting Bubble Tea’s Elm Architecture.

---

## Core Abstractions (Shared Across Options)

All options build on these primitives:

| Abstraction | Purpose |
|-------------|---------|
| **View** | A screen or major UI region with its own model, update, view |
| **Panel** | A bounded region within a layout (can host a View) |
| **Focus** | Which View/Panel receives input; single global focus |
| **Layout** | How Panels are arranged (split, stack, overlay) |

---

## Option A: View Stack + Project Workspace

**Idea**: One active project at a time. Stack of views (e.g., project list → project detail → PR detail). Artifacts live in a collapsible panel or modal.

```
┌─────────────────────────────────────────────────────────┐
│ [Project: HA sampler querier]  [Artifacts ▼]             │
├─────────────────────────────────────────────────────────┤
│ ┌─────────────────────┬───────────────────────────────┐ │
│ │ Repos (2)            │ PRs / Issues (12)             │ │
│ │ • repo-a             │ • PR #42 [in review]          │ │
│ │ • repo-b             │ • PR #41 [merged]             │ │
│ │                      │ • Issue #38                   │ │
│ │                      │ ...                           │ │
│ └─────────────────────┴───────────────────────────────┘ │
│ [SPC] for commands                                       │
└─────────────────────────────────────────────────────────┘
```

**Model shape**:
```go
type AppModel struct {
    ViewStack   []View          // Stack of pushed views
    Workspace   ProjectWorkspace // Current project: repos, PRs
    Artifacts   ArtifactPanel   // Agent plan, design doc (collapsible)
    Focus       FocusTarget     // Which panel has focus
}
```

**Pros**: Simple mental model; clear “one project at a time”
**Cons**: Switching projects requires navigation; artifacts not always visible

---

## Option B: Multi-Project Split with Artifact Rail

**Idea**: Show 2–3 projects side-by-side. Persistent “artifact rail” on one edge for the active project’s plan/design doc.

```
┌──────────────────────────────────────────────────────────────────┐
│ [HA sampler querier] [Other project] [Other project]   │ Plan │  │
├──────────────────────────────────────────────────────────────────┤
│ ┌─────────────────┬─────────────────┬─────────────────┐ │      │  │
│ │ Project 1       │ Project 2       │ Project 3       │ │Agent │  │
│ │ Repos | PRs     │ Repos | PRs     │ Repos | PRs     │ │plan  │  │
│ │                 │                 │                 │ │      │  │
│ │ (focused)       │                 │                 │ │Design│  │
│ └─────────────────┴─────────────────┴─────────────────┘ │doc   │  │
└──────────────────────────────────────────────────────────────────┘
```

**Model shape**:
```go
type AppModel struct {
    Projects    []ProjectModel   // 2–3 project panes
    ArtifactRail ArtifactRail    // Sticky: plan + design doc for focused project
    Focus       FocusTarget      // Which project pane, or rail
    Layout      SplitLayout     // H-splits for projects, rail width
}
```

**Pros**: Cross-project visibility; artifacts always visible
**Cons**: More complex layout; smaller per-project area

---

## Option C: Project Tabs + Context Panel

**Idea**: Tabs for projects. Main area shows selected project’s content. Bottom or side “context panel” holds artifacts for the active project.

```
┌─────────────────────────────────────────────────────────┐
│ [HA sampler] [Project B] [Project C]                     │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  Main content: repos, PRs, issues for selected project  │
│                                                         │
├─────────────────────────────────────────────────────────┤
│ Context (always visible): Agent plan | Design doc       │
│ [Excerpt or full - toggle with key]                     │
└─────────────────────────────────────────────────────────┘
```

**Model shape**:
```go
type AppModel struct {
    Tabs        TabBar           // Project tabs
    MainView    View             // Content for selected project
    ContextPanel ContextPanel    // Artifacts; resizable height
    Focus       FocusTarget      // Tab bar, main, or context
}
```

**Pros**: Familiar tab metaphor; artifacts always present
**Cons**: Context panel competes for vertical space

---

## Option D: Workspace + Overlay Stack

**Idea**: One workspace (project) visible. Artifacts as overlays (modals) that can be pinned “always on top” or toggled. Good for “include in every session” behavior.

```
┌─────────────────────────────────────────────────────────┐
│ Workspace: HA sampler querier                           │
├─────────────────────────────────────────────────────────┤
│ ┌─────────────────────────────────────────────────────┐ │
│ │ Repos (2)  │  PRs (12)  │  [Artifacts: Plan, Doc]   │ │
│ │            │            │  (click to overlay)       │ │
│ └─────────────────────────────────────────────────────┘ │
│                                                         │
│  ┌───────────────────────────────┐  ← Overlay (optional)│
│  │ Agent Plan (pinned)           │                       │
│  │ ...                          │                       │
│  └───────────────────────────────┘                       │
└─────────────────────────────────────────────────────────┘
```

**Model shape**:
```go
type AppModel struct {
    Workspace   ProjectWorkspace
    Overlays    []Overlay        // Stack of overlays (plan, doc, etc.)
    Pinned      []OverlayID      // Which overlays stay visible
    Focus       FocusTarget      // Workspace or top overlay
}
```

**Pros**: Flexible; artifacts on demand or pinned
**Cons**: Overlays can clutter; focus management more complex

---

## Option E: Dashboard + Detail (Recommended Starting Point)

**Idea**: Dashboard lists all projects with summary. Selecting a project opens a detail view with splits for repos/PRs and a dedicated artifact area. Balances visibility and simplicity.

```
Dashboard:
┌─────────────────────────────────────────────────────────┐
│ Projects (3)                              [SPC] commands │
├─────────────────────────────────────────────────────────┤
│ ● HA sampler querier    2 repos, 12 PRs, 2 artifacts    │
│   Project B             1 repo,  5 PRs, 1 artifact      │
│   Project C             3 repos, 8 PRs, 0 artifacts     │
└─────────────────────────────────────────────────────────┘

Project Detail (after selecting HA sampler):
┌─────────────────────────────────────────────────────────┐
│ ← HA sampler querier                    [Plan] [Design]  │
├──────────────────────┬──────────────────────────────────┤
│ Repos                │ PRs / Issues                      │
│ • repo-a             │ #42 in review                     │
│ • repo-b             │ #41 merged                        │
│                      │ ...                               │
├──────────────────────┴──────────────────────────────────┤
│ Artifact: Agent plan (excerpt) — [e] expand, [SPC] open  │
└─────────────────────────────────────────────────────────┘
```

**Model shape**:
```go
type AppModel struct {
    Mode       AppMode           // Dashboard | ProjectDetail
    Dashboard  DashboardModel    // List of projects + summaries
    Detail     ProjectDetailModel // Selected project: splits + artifacts
    Artifacts  ArtifactStore    // Plan, design doc per project
    Focus      FocusTarget
}
```

**Pros**: Clear hierarchy; artifacts integrated per project; scales to many projects
**Cons**: One extra navigation step (dashboard → detail)

---

## Abstraction Layer Design (Implementation-Oriented)

Regardless of option, the following interfaces support composition:

```go
// View is the unit of composition; implements Bubble Tea's Init/Update/View
type View interface {
    Init() tea.Cmd
    Update(tea.Msg) (View, tea.Cmd)
    View() string
}

// Panel hosts a View and knows its bounds
type Panel struct {
    ID    string
    View  View
    Bounds func(width, height int) (x, y, w, h int)
}

// Layout arranges panels
type Layout interface {
    Panels() []Panel
    FocusOrder() []string  // Tab order for focus
}

// FocusManager tracks and rotates focus
type FocusManager struct {
    Current  string
    Order    []string
    OnChange func(from, to string)
}
```

**View stack** (for navigation):
```go
type ViewStack struct {
    Stack []View
    Push(v View) ViewStack
    Pop() (View, ViewStack)
    Peek() View
}
```

**Overlay** (for modals, artifact popups):
```go
type Overlay struct {
    View   View
    Dismiss tea.Key // e.g. Esc
}
```

---

## Recommendation

**Start with Option E (Dashboard + Detail)** because it:

1. Matches the project-centric workflow (2–3 projects, many PRs)
2. Gives artifacts a clear place (per-project, always accessible)
3. Keeps the model simple (Dashboard vs Detail mode)
4. Allows later evolution toward Option B (multi-project split) or Option C (tabs) without changing core abstractions

**Next steps**:
- Implement `View`, `Panel`, `Layout`, `FocusManager` interfaces
- Implement `ViewStack` for navigation
- Implement `Overlay` for modals
- Prototype Dashboard + ProjectDetail models using these primitives

---

## Resolved: Artifacts & Persistence

### Artifacts (Q1, Q2)
- **Files in a well-known location** — artifacts (agent plan, design doc) are plain files on disk
- Editable with any tool (editor, agents, CLI)
- "Include in every session" = agents link to the file path; no special metadata needed

### Project directory layout
- **One directory per project** — e.g. `~/.devdeploy/projects/<project-name>/`
- **Worktrees inside** — each repo gets a worktree subdir (e.g. `repo-a/`, `repo-b/`)
- **Config + artifacts co-located** — `config.yaml`, `plan.md`, `design.md` live in the project root

```
~/.devdeploy/projects/
  ha-sampler-querier/
    config.yaml       # project config (repos, artifact refs, etc.)
    plan.md           # agent plan
    design.md         # design doc
    repo-a/           # worktree for first repo
    repo-b/           # worktree for second repo
  other-project/
    config.yaml
    ...
```

- No separate project→path mapping; the directory structure is the source of truth
- Add SQLite later only if we need richer state (history, PR cache, etc.)
