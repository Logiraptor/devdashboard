# SPC p Project Management Keybinds — Validation Checklist

**Date**: 2026-02-06  
**Related**: dev-log/2026-02-06-project-management-keybinds.md, beads devdeploy-i1u.16

## Prerequisites

- Run `go run ./cmd/devdeploy` in a terminal with TTY
- Ensure `~/.devdeploy/projects/` exists (or set `DEVDEPLOY_PROJECTS_DIR`)
- For add/remove repo: have git repos in `~/workspace` (or `DEVDEPLOY_WORKSPACE`)

## Checklist

### Dashboard mode

| Keybind | Action | Expected |
|---------|--------|----------|
| `SPC p c` | Create project | Modal appears; Enter creates project, Esc cancels |
| `SPC p d` | Delete selected project | Selected project is deleted; list refreshes |
| `SPC p a` | Add repo | No-op (silent; add is Project detail only) |
| `SPC p r` | Remove repo | No-op |

### Project detail mode (after selecting a project)

| Keybind | Action | Expected |
|---------|--------|----------|
| `SPC p c` | Create project | Modal appears (create allowed from any mode) |
| `SPC p d` | Delete project | No-op (delete is Dashboard-only) |
| `SPC p a` | Add repo | Repo picker modal; lists ~/workspace repos; Enter adds worktree |
| `SPC p r` | Remove repo | Repo picker modal; lists project repos; Enter removes worktree |

### Help view

- After `SPC`, hints show `p`, `q`, `a`
- After `SPC p`, hints show `c`, `d`, `a`, `r`

## Automated tests

- `go test ./internal/ui/... -run TestProjectKeybinds` — message handling and context rules
- `go test ./internal/ui/... -run TestSPC` — SPC shows keybind hints (`TestSPCShowsKeybindHints`), commands execute (`TestSPCKeybindCommandsExecute`)
