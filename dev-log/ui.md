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
- Dynamic height (5-15 lines) based on terminal size, consistent within session to prevent layout jumping

**Content displayed**:
- **Title**: Bead ID and title (e.g., `devdeploy-abc  Fix the thing`)
- **Status**: Status and issue type (e.g., `in_progress  task`)
- **Description**: Full description text, shows as much as will fit (calculated from section height)
- **Labels**: Comma-separated list of labels, truncated if too long

**Implementation**:
- `BeadInfo` struct (`internal/project/resource.go`) includes `Description` and `Labels` fields
- Rendered by `renderBeadDetailsSection()` in `internal/ui/projectdetail.go`
- Uses `SelectedBead()` to determine which bead to display

### History

The artifact system (plan.md / design.md) was removed in 2026-02-08 (see `devdeploy-lvr` epic). Beads integration replaced it as the primary way to track work items per resource.

## Multi-Agent Parallel TUI

**Status**: accepted  
**Last updated**: 2026-02-16

The multi-agent parallel TUI displays multiple agent blocks simultaneously, allowing users to monitor concurrent agent activity across multiple beads.

### Components

#### MultiAgentView

`MultiAgentView` (`internal/ralph/tui/multi_agent_view.go`) manages the grid layout of multiple agent blocks:

- **Agent tracking**: Maintains a map of `AgentBlock` instances keyed by bead ID, preserving display order
- **Thread-safe**: Uses `sync.RWMutex` for concurrent access from agent goroutines
- **Summary stats**: Tracks counts of succeeded, failed, and question-status agents
- **Layout**: Automatically switches between single-column and two-column layouts based on terminal width

**Key methods**:
- `StartAgent(bead)` — Begins tracking a new agent for a bead
- `CompleteAgent(beadID, status)` — Marks an agent as complete and updates summary stats
- `AddToolEvent(beadID, toolName, started, attrs)` — Adds tool events to an agent's stream
- `View()` — Renders the grid layout
- `Summary()` — Returns formatted summary line (e.g., "2 running | 1 done | 1 failed | 1 questions")

#### AgentBlock

`AgentBlock` (`internal/ralph/tui/agent_block.go`) represents a single agent's status and activity stream:

- **Status tracking**: States include `running`, `success`, `failed`, `timeout`, `question`
- **Event stream**: Ring buffer of last 4 events (`MaxEvents = 4`) showing tool invocations
- **Visual indicators**: 
  - Animated spinner (⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏) for running agents
  - Status icons: ✓ (success), ✗ (failed/timeout), ? (question)
  - Color-coded borders based on status (blue=running, green=success, red=failed, yellow=question)
- **Tool details**: Extracts and displays relevant details from tool attributes:
  - File paths for Read/Write operations (shortened)
  - Commands for Shell/Bash (truncated to 50 chars)
  - Query/pattern for Grep/Search (truncated to 40 chars)
- **Duration display**: Shows elapsed time for running agents, final duration for completed ones

**Event rendering**: Each event shows:
- Tool icon (Unicode symbols: ◀ Read, ▶ Write, ⬢ Shell, ◉ Grep, etc.)
- Tool name (styled)
- Detail (file path, command, query, etc.)
- Duration for completed tools

### Layout Behavior

**Single-column layout** (width ≤ 120):
- Agent blocks stacked vertically
- Full terminal width minus 2 chars padding
- Minimum block width: 50 chars

**Two-column layout** (width > 120):
- Agent blocks arranged in pairs side-by-side
- Block width: `(width - 4) / 2`
- Odd-numbered agents: last block spans full width
- Uses `lipgloss.JoinHorizontal` for side-by-side rendering

**Block structure**:
```
┌─────────────────────────────────────┐
│ ✓ bead-id  Title text        1m23s │  ← Header: icon, ID, title, duration
│   ◀ Read  internal/file.go         │  ← Event 1
│   ▶ Write  config.yaml              │  ← Event 2
│   ⬢ Shell  git push origin main    │  ← Event 3
│   ·                                 │  ← Event 4 (placeholder if < 4 events)
└─────────────────────────────────────┘
```

### Summary Statistics

The `Summary()` method aggregates and formats agent status:

- **Running**: Count of agents with `status == "running"`
- **Done**: Count of agents with `status == "success"`
- **Failed**: Count of agents with `status == "failed"` or `"timeout"`
- **Questions**: Count of agents with `status == "question"`

Format: `"2 running | 1 done | 1 failed | 1 questions"` (omits zero counts)

### Styling

Uses Catppuccin-inspired color palette:
- **Borders**: Status-based colors (blue/green/red/yellow)
- **Bead ID**: Bold mauve (`#cba6f7`)
- **Title**: Subtext1 (`#bac2de`)
- **Tool names**: Pink (`#f5c2e7`)
- **Event details**: Subtext0 (`#a6adc8`)
- **Icons**: Lavender (`#b4befe`)

### Thread Safety

All public methods use appropriate locking:
- `SetSize`, `StartAgent`, `CompleteAgent`, `AddToolEvent`, `UpdateDuration` — write lock
- `View`, `Summary`, `GetActiveBeadID`, `ActiveCount`, `TotalCount` — read lock

This allows safe concurrent updates from multiple agent goroutines while rendering occurs on the main TUI thread.
