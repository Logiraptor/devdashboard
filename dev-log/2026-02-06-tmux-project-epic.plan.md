# DevDeploy: Real PRs, Selection, Keybind Fix, and Tmux Organization

**Date**: 2026-02-06
**Status**: proposed

## Overview

Next phase of devdeploy development: replace fake PR data with real API integration, add selection and shell actions for repos/PRs, fix the shell keybind (SPC a a → SPC s s), and implement automatic tmux organization with devdeploy as a persistent control panel.

---

## 1. Real PR Lists in Projects

**Current state**: `ProjectDetailView` hardcodes `PRs: []string{"#42 in review", "#41 merged", "#38 open"}`. `project.Manager.CountPRs` returns 0 with a TODO for gh integration. `newProjectDetailView` never populates PRs from any real source.

**Goal**: Integrate with `gh pr list` (or similar) per repo in the project. Show real open PRs, optionally with status (open, merged, in review). Populate both the project detail view and the dashboard PR count.

**Clarifications**:
- Use `gh` CLI only, or support other providers (GitLab API, etc.)?
- Show only open PRs, or also recently merged?
- Group PRs by repo in the UI?

---

## 2. Selection and Shell Actions in Projects

**Current state**: Project detail shows repos and PRs as static lists. No selection (j/k), no actions. `SPC s s` (currently SPC a a) opens a shell in the project directory only.

**Goal**: 
- Make repos and PRs selectable (j/k navigation, highlight selected)
- **Open shell in repo worktree**: Select a repo, trigger action → shell opens in that repo's project worktree path
- **Open shell with PR checked out**: Select a PR, trigger action → `gh pr checkout <n>` (or equivalent) in the repo, then open shell there

**Clarifications**:
- Single list (repos + PRs interleaved) or separate selectable sections (Repos section, PRs section)?
- For "shell with PR checked out": run `gh pr checkout` in the worktree, then split pane with that cwd?
- New keybinds: e.g. `SPC s s` on selected repo = shell in worktree; `SPC s p` on selected PR = shell with PR checked out? Or Enter to open shell for selected item?

---

## 3. Keybind: SPC a a → SPC s s

**Current state**: `SPC a a` = "Agent run" (opens tmux pane with shell). Descriptions say "Agent run".

**Goal**: Rename to `SPC s s` because it opens a shell. Update hints to say "Open shell" or "Shell" instead of "Agent run".

**Submenu structure**:
- `SPC s` = Shell submenu
- `SPC s s` = Open shell (primary action)
- `SPC s h` = Hide shell pane (was SPC a h)
- `SPC s j` or `SPC s a` = Show/join shell pane (was SPC a s)

**Files to update**: `app.go` (bindings), `keybind.go` (firstLevelSubmenuLabel), `keybinds.md`, `agent-workflow.md`, tests.

---

## 4. Tmux Organization: Project Sections, DevDeploy Control Panel

**Current state**: devdeploy runs in one pane. `SPC s s` creates a new pane via `tmux split-window -c <projectDir>`. No project-specific layout. Panes are ad-hoc.

**Goal**:
- **DevDeploy always visible on the left** as a control panel
- **Each project gets a separate section** in tmux (e.g. window or named pane group)
- **Selecting a project** switches the right side to that project's space
- **Panes created in the project** (e.g. SPC s s) end up in that project's area, not mixed with devdeploy

**Proposed layout** (to confirm):
- **Option A**: Single window, split: `[devdeploy | project panes]`. Left pane = devdeploy (fixed). Right = stack of panes for current project. Switching project = swap right panes (e.g. break current project panes to background windows, join target project's panes).
- **Option B**: Multiple windows: window 0 = devdeploy + project A panes; window 1 = devdeploy + project B panes. Selecting project = switch window. (DevDeploy would need to run in each window or we use a different approach.)
- **Option C**: Single window, two-pane layout: left = devdeploy, right = current project. Right side is one window per project; `select-window` switches. New shells go to current project's window.

**Clarifications**:
- Preferred layout: A, B, C, or something else?
- When switching projects, should previous project's shells persist (break to background, restore on switch back)?
- Should devdeploy create the layout on startup if it doesn't exist?

---

## Ideas for Further Improvements

1. **Window/pane naming**: Name tmux windows by project (e.g. "devdeploy", "my-project"). Pane titles show repo or PR.
2. **Quick project switch**: `SPC p w` to switch project window without going through dashboard.
3. **Persistent layout**: On devdeploy start, create or restore the standard layout (devdeploy left, project area right).
4. **Per-project shell persistence**: When switching back to a project, its shell panes are still there (break-pane to background window, join-pane on return).
5. **SPC s submenu expansion**: `SPC s s` = shell in project dir; `SPC s r` = shell in selected repo worktree; `SPC s p` = shell with selected PR checked out (once selection exists).
6. **Dashboard PR count**: Wire `CountPRs` to real `gh pr list` so dashboard shows accurate counts.
