# Agent Workflow

**Status**: accepted  
**Last updated**: 2026-02-25

## Overview

Agent workflow spans progress streaming, live display, abort capability, and shell orchestration. The app uses **tmux session orchestration** (not embedded PTY) for interactive resource shells.

## Resource-based Model (Current Direction)

As of V2, interaction is session-centric. A resource owns at most one tmux session, and devdeploy tracks those sessions directly. The workflow is:

1. User selects a **resource** (repo, worktree, or PR) in the home view
2. `enter` → creates worktree if needed → ensures tmux session for the selected resource
3. User is switched into that resource session to work directly in shell/agent tooling
4. devdeploy tracks the session by typed resource key and can preview output with `capture-pane`

Navigation uses a two-level cursor: `j`/`k` move through beads within a resource before advancing to the next resource header.

This replaced the earlier `AgentRunner` interface / `StubRunner` / progress event stream approach for agent execution. The progress/abort infrastructure remains for potential future use but is not the primary agent interaction model.

See `devdeploy-7uj` epic for full details.

## Phase 5: Integration (Revised)

> **Note**: The original Phase 5 included an Artifact Store (plan.md / design.md) subsection. Artifacts were removed in 2026-02-08 (`devdeploy-lvr` epic) in favor of **beads integration** — see [ui.md](ui.md#beads-per-resource). The Agent Runner interface no longer references plan/design paths.

### Progress Event Stream

- `progress.Event` type: message, status, timestamp, optional metadata
- Emission: channel + `tea.Msg` (ChanEmitter)
- `internal/progress` package to avoid ui↔agent import cycle

### Agent Runner Interface

```go
type AgentRunner interface {
    Run(ctx context.Context, projectDir string) (tea.Cmd or chan ProgressEvent)
}
```

- `enter` opens or switches to the selected resource session
- Stub implementation emits fake progress events for integration testing

## Phase 6: Live Progress Windows

- **ProgressWindow** overlay: displays `progress.Event` stream with timestamps and status icons (● running, ✓ done, ✗ error)
- Uses `bubbles/viewport` for scrollback (j/k, pgup/pgdown)
- Shown while progress events are active; dismissed with Esc

## Phase 7: Abort

- `RunAgentMsg` creates `context.WithCancel`; cancel func stored on `AppModel`
- Runner checks `ctx.Done()` during blocking work
- Esc on ProgressWindow overlay triggers cancel; emits `StatusAborted` (⊗ icon)
- When run completes (Done or Aborted), cancel func cleared

## Tmux Session Orchestration (Current Approach)

**Requires tmux** — App expects `TMUX` env. If unset, shows message to start tmux first.

1. **Session lifecycle**: `enter` creates or reuses a session named from the selected resource key.
2. **Switching**: `tmux switch-client -t <session>` moves the user into that resource session.
3. **Cleanup**: `SPC s k` kills the selected resource session and unregisters it from tracker state.
4. **Preview**: periodic tick refresh captures recent pane output (`capture-pane`) for the selected session and renders it in Home.

**Rationale**: Native tmux pane = full terminal features, no key translation, simpler code. PTY embedding competed with tmux when users ran devdeploy inside tmux.

## PTY Approach (Deprecated)

Embedded PTY (`ShellView` + `internal/pty`) is **deprecated**. Superseded by tmux session orchestration. PTY package retained for tests or future non-tmux scenarios.

## Validation Checklists

### Agent progress and abort

1. Run `go run ./cmd/devdeploy` in TTY; select a repo/resource
2. `enter` on a resource → switched into tmux session
3. Wait for stub output → events stream with timestamps and icons
4. Esc during run → aborts; "Aborted" with ⊗ icon
5. Esc again → overlay dismisses

**Tests**: `go test ./internal/ui/... -run TestAgentProgressVisible`, `TestAgentAbort`; `go test ./internal/agent/... -run TestEmitAfter`

### Session and resource management

1. Home: `SPC p a` → add worktree (branch prompt)
2. Home: `SPC p x` (or `d`) → remove selected resource
3. Home: `SPC s k` kills selected resource session; `SPC s c` opens Cursor in selected worktree

**Tests**: `go test ./internal/ui/... -run TestSPC`
