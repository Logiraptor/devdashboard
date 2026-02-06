# Project Management Keybinds and Real Data

**Date**: 2026-02-06
**Status**: accepted

## Context

Phase 5.1 (Artifact store) already loads plan/design from disk. The dashboard still showed placeholder projects. The user wanted to switch to real data and add project management keybinds with SPC leader integration.

## Decision

### Real Data from Disk

- **Dashboard**: Loads projects from `~/.devdeploy/projects/` via `ProjectsLoadedMsg` on init
- **Project detail**: Repos come from worktree subdirs in the project; artifacts from plan.md, design.md
- **Project list**: Scans directory; each subdir is a project (name = dir name, e.g. `ha-sampler-querier`)

### Project Management Package (`internal/project`)

- `ListProjects()` — scan `~/.devdeploy/projects/`
- `CreateProject(name)` — mkdir + config.yaml
- `DeleteProject(name)` — rm -rf
- `ListWorkspaceRepos()` — git repos in `~/workspace` (or `DEVDEPLOY_WORKSPACE`)
- `ListProjectRepos(name)` — worktree subdirs in project
- `AddRepo(project, repo)` — `git worktree add` from ~/workspace/repo into project dir
- `RemoveRepo(project, repo)` — `git worktree remove`

### SPC p Keybinds

| Sequence | Action | Context |
|----------|--------|---------|
| `SPC p c` | Create project | Dashboard |
| `SPC p d` | Delete selected project | Dashboard |
| `SPC p a` | Add repo to project | Project detail |
| `SPC p r` | Remove repo from project | Project detail |

### Multi-Level Leader

- KeyHandler supports sequences like `SPC p c`: after `SPC p`, stays in leader mode if `HasPrefix("SPC p")`
- Help view shows next-level hints (e.g. after `SPC p`, shows `c`, `d`, `a`, `r`)

### Modals

- **Create project**: Text input; Enter creates, Esc cancels
- **Delete project**: Confirmation modal; shows project name; y/Enter confirms, Esc cancels (Phase 7 destructive-action principle)
- **Add repo**: List picker of ~/workspace repos; Enter adds worktree, Esc cancels
- **Remove repo**: List picker of project repos; Enter removes worktree, Esc cancels

## Implementation

- `internal/project/project.go`: Manager with CRUD and worktree ops
- `internal/ui/modal_create_project.go`: CreateProjectModal (bubbles textinput)
- `internal/ui/modal_delete_project.go`: DeleteProjectConfirmModal (confirmation)
- `internal/ui/modal_repo_picker.go`: RepoPickerModal (bubbles list)
- `internal/ui/keybind.go`: HasPrefix, multi-level leader, LeaderHints(prefix)
- `internal/ui/app.go`: Overlay stack, message handlers, loadProjectsCmd

## Consequences

- Dashboard shows real projects; empty if none exist
- User can create projects, add repos (worktrees), remove repos
- SPC p submenu integrates with help view
- `DEVDEPLOY_WORKSPACE` env overrides ~/workspace for repo listing
