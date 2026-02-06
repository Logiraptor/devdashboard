# DevDeploy: Real PRs, Selection, Keybind Fix, and Tmux Organization

**Date**: 2026-02-06
**Status**: accepted

## Overview

Next phase of devdeploy development: replace fake PR data with real API integration, add selection and shell actions for repos/PRs, fix the shell keybind (SPC a a → SPC s s), and implement automatic tmux organization with devdeploy as a persistent control panel.

---

## 1. Real PR Lists in Projects

**Current state**: `ProjectDetailView` hardcodes `PRs: []string{"#42 in review", "#41 merged", "#38 open"}`. `project.Manager.CountPRs` returns 0 with a TODO for gh integration. `newProjectDetailView` never populates PRs from any real source.

**Goal**: Integrate with `gh pr list` per repo in the project. Populate both the project detail view and the dashboard PR count.

**Decisions**:
- **gh only** — No GitLab or other providers
- **Open + recently merged** — Show both open and recently merged PRs
- **Group by repo** — PRs displayed grouped by repo in the UI
- **Wire CountPRs** — Dashboard PR count uses real `gh pr list` numbers

---

## 2. Selection and Shell Actions in Projects

**Current state**: Project detail shows repos and PRs as static lists. No selection (j/k), no actions. `SPC s s` (currently SPC a a) opens a shell in the project directory only.

**Goal**:
- Make repos and PRs selectable (j/k navigation, highlight selected)
- **Enter on selected item** opens shell: repo = shell in worktree current state; PR = `gh pr checkout <n>` in that repo's worktree, then open shell there

**Decisions**:
- **Repos separate from PRs** — Two selectable sections: Repos, PRs (grouped by repo)
- **Enter = open shell** — Enter on repo → shell in worktree; Enter on PR → checkout PR branch, then shell
- **Checkout then open** — For PR: run `gh pr checkout` in the worktree, then split pane with that cwd

---

## 3. Keybind: SPC a a → SPC s s

**Current state**: `SPC a a` = "Agent run" (opens tmux pane with shell). Descriptions say "Agent run".

**Goal**: Rename to `SPC s s` because it opens a shell. Update hints to say "Open shell" or "Shell" instead of "Agent run".

**Submenu structure**:
- `SPC s` = Shell submenu
- `SPC s s` = Open shell (primary action; project dir when nothing selected)
- `SPC s h` = Hide shell pane (was SPC a h)
- `SPC s j` or `SPC s a` = Show/join shell pane (was SPC a s)

**Files to update**: `app.go` (bindings), `keybind.go` (firstLevelSubmenuLabel), `keybinds.md`, `agent-workflow.md`, tests.

---

## 4. Tmux Organization: Project Sections, DevDeploy Control Panel

**Current state**: devdeploy runs in one pane. `SPC s s` creates a new pane via `tmux split-window -c <projectDir>`. No project-specific layout. Panes are ad-hoc.

**Goal**:
- **DevDeploy always visible on the left** as a control panel
- **Each project gets a separate section** in tmux
- **Selecting a project** switches the right side to that project's space
- **Panes created in the project** end up in that project's area

**Decisions**:
- **Layout: Option C** — Single window, two-pane layout: left = devdeploy, right = current project. Right side is one window per project; `select-window` switches. New shells go to current project's window.
- **Persistent shells** — When switching projects, break current project's panes to background; restore (join-pane) when switching back
- **Init on startup** — devdeploy creates the layout on startup if it doesn't exist

---

## In Scope (from Ideas)

1. **Quick project switch**: `SPC p l` — Switch project window without going through dashboard
2. **Persistent shells**: Break to background on project switch, join on return (see §4)
3. **Wire PR numbers**: `CountPRs` uses real `gh pr list` (see §1)
4. **Window/pane naming**: Name tmux windows by project; pane titles show repo or PR
