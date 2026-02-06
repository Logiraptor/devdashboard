# Tmux Pane Orchestration for Agent Shells

**Date**: 2026-02-06
**Status**: accepted
**Epic**: devdeploy-bgt

## Context

The previous agent flow used an embedded PTY (`ShellView` + `internal/pty`) to spawn a shell inside the Bubble Tea TUI. Engineers needed a real interactive shell for agents (Cursor, Claude Code), but embedding a PTY in a TUI overlay introduced complexity: key translation (`keyToPTYBytes`), viewport rendering of PTY output, and focus/escape handling. The PTY approach also competed with tmux for terminal control when users ran devdeploy inside tmux.

## Decision

Use **tmux pane orchestration** instead of embedding a PTY:

1. **Require tmux** — The app expects to run inside tmux (`TMUX` env set). If unset, show a clear message directing the user to start tmux first.

2. **Pane layout** — devdeploy (Bubble Tea TUI) runs in one pane. On "Agent run" (SPC a a), the app creates a new pane via `tmux split-window -c <workDir>` with a shell in the project directory. No PTY in our code.

3. **internal/tmux package** — Provides: `SplitPane(workDir)`, `KillPane(paneID)`, `SendKeys(paneID, keys)`, `BreakPane(paneID)`, `JoinPane(paneID)`. All operations use `exec.Command("tmux", ...)` targeting the current session.

4. **Hide/show** — "Hide" agent pane: `break-pane -d` moves it to a background window. "Show": `join-pane` restores it. Pane ID is tracked in app state.

5. **PTY package** — Retained for tests or future non-tmux scenarios; marked as optional/fallback. ShellView and PTY are deprecated from the agent flow.

## Consequences

- Engineers get a native tmux pane for agent shells — full terminal features, no key translation, tmux handles rendering.
- Simpler code: no PTY capture, viewport, or key mapping in the app.
- Requires tmux; users must run devdeploy inside a tmux session.
- Pane lifecycle (split, kill, break, join) is explicit and testable via `internal/tmux` tests (skipped when not in tmux).
