# Embedded PTY/Shell for Agent Interaction

**Date**: 2026-02-06
**Status**: deprecated (superseded by tmux pane orchestration)
**Task**: devdeploy-awh.1 (closed)
**Superseded by**: devdeploy-bgt (tmux pane orchestration)

## Context

The previous agent flow used a `ProgressWindow` overlay that displayed `progress.Event` stream from `StubRunner`. For interactive agents (Cursor, Claude Code), engineers need a **real PTY-backed shell** so they can type, run commands, and interact directly. The progress overlay was replaced with an embedded shell.

## Decision

1. **PTY abstraction** — `internal/pty` package with:
   - `Runner` interface: `Start(ctx, cmd, size) (io.ReadWriteCloser, error)` and `Resize(rwc, size) error`
   - `CreackPTY` implementation wrapping `github.com/creack/pty`
   - Dependency injection so the UI can swap implementations (e.g. for tests)

2. **ShellView** — PTY-backed overlay that:
   - Spawns a shell (bash or sh) in the project directory
   - Passes keyboard input to the PTY (KeyMsg → bytes via `keyToPTYBytes`)
   - Displays PTY output in a viewport with scrollback
   - Esc dismisses (does not pass through to shell)

3. **RunAgentMsg wiring** — Replaced `ProgressWindow` + `AgentRunner.Run` with `ShellView` + PTY. The shell runs in the project directory.

## Consequences

- Engineers get an interactive shell when running SPC a a from project detail
- PTY interface allows swapping libraries or mocking for tests
- ProgressWindow and StubRunner remain in codebase for potential future use (e.g. non-interactive agent runs)

## Deprecation (2026-02-06)

This approach is **deprecated**. Use **tmux pane orchestration** (devdeploy-bgt) instead: app runs in one pane, creates shells via `tmux split-window`. See devdeploy-bgt epic.
