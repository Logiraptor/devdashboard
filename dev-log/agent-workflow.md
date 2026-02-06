# Agent Workflow

**Status**: accepted  
**Last updated**: 2026-02-06

## Overview

Agent workflow spans artifact storage, progress streaming, live display, abort capability, and shell orchestration. The app uses **tmux pane orchestration** (not embedded PTY) for interactive agent shells.

## Phase 5: Integration

### Artifact Store

- `ArtifactStore` reads `plan.md`, `design.md` from `~/.devdeploy/projects/<name>/`
- `ProjectDetailView` consumes real content from app model
- Missing files → empty or "no plan yet"

### Progress Event Stream

- `progress.Event` type: message, status, timestamp, optional metadata
- Emission: channel + `tea.Msg` (ChanEmitter)
- `internal/progress` package to avoid ui↔agent import cycle

### Agent Runner Interface

```go
type AgentRunner interface {
    Run(ctx context.Context, projectDir, planPath, designPath string) (tea.Cmd or chan ProgressEvent)
}
```

- `SPC s s` triggers agent run (opens shell) for current project
- Stub implementation emits fake progress events for integration testing

## Phase 6: Live Progress Windows

- **ProgressWindow** overlay: displays `progress.Event` stream with timestamps and status icons (● running, ✓ done, ✗ error)
- Uses `bubbles/viewport` for scrollback (j/k, pgup/pgdown)
- Shown when user triggers `SPC s s`; dismissed with Esc

## Phase 7: Abort

- `RunAgentMsg` creates `context.WithCancel`; cancel func stored on `AppModel`
- Runner checks `ctx.Done()` during blocking work
- Esc on ProgressWindow overlay triggers cancel; emits `StatusAborted` (⊗ icon)
- When run completes (Done or Aborted), cancel func cleared

## Tmux Pane Orchestration (Current Approach)

**Requires tmux** — App expects `TMUX` env. If unset, shows message to start tmux first.

1. **Layout init on startup**: devdeploy creates a two-pane layout if it doesn't exist: left = devdeploy (control panel), right = project area. `tmux.EnsureLayout()` splits horizontally when the window has only one pane.
2. **Pane layout**: devdeploy runs in the left pane. `SPC s s` creates new pane via `tmux split-window -c <workDir>` with shell in project directory.
3. **internal/tmux**: `EnsureLayout`, `WindowPaneCount`, `SplitPane(workDir)`, `KillPane`, `SendKeys`, `BreakPane`, `JoinPane`
4. **Hide/show**: `break-pane -d` moves agent pane to background window; `join-pane` restores it.

**Rationale**: Native tmux pane = full terminal features, no key translation, simpler code. PTY embedding competed with tmux when users ran devdeploy inside tmux.

## PTY Approach (Deprecated)

Embedded PTY (`ShellView` + `internal/pty`) is **deprecated**. Superseded by tmux pane orchestration. PTY package retained for tests or future non-tmux scenarios.

## Validation Checklists

### Agent progress and abort

1. Run `go run ./cmd/devdeploy` in TTY; create project; select it
2. `SPC s s` → ProgressWindow (or tmux pane) appears
3. Wait for stub output → events stream with timestamps and icons
4. Esc during run → aborts; "Aborted" with ⊗ icon
5. Esc again → overlay dismisses

**Tests**: `go test ./internal/ui/... -run TestAgentProgressVisible`, `TestAgentAbort`; `go test ./internal/agent/... -run TestEmitAfter`

### SPC p project management

1. Dashboard: `SPC p c` → create modal; `SPC p d` → delete selected
2. Project detail: `SPC p a` → add repo picker; `SPC p r` → remove repo picker
3. Help: After `SPC` → hints show `p`, `q`, `s`; after `SPC p` → `c`, `d`, `a`, `r`

**Tests**: `go test ./internal/ui/... -run TestProjectKeybinds`, `TestSPC`
