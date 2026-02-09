# Agent Workflow

**Status**: accepted  
**Last updated**: 2026-02-08

## Overview

Agent workflow spans progress streaming, live display, abort capability, and shell orchestration. The app uses **tmux pane orchestration** (not embedded PTY) for interactive agent shells.

## Resource-based Model (Current Direction)

As of 2026-02-07, the agent concept is simplified: **an agent is just a shell with a predefined command** (`agent` — Cursor's CLI). There is no separate "agent runner" abstraction. The workflow is:

1. User selects a **resource** (repo or PR) in a project
2. `SPC s a` → creates worktree if needed → splits tmux pane → runs `agent` in that pane
3. User interacts with the agent directly in its native interface
4. devdeploy tracks the pane as type "agent" in the session tracker

### Targeted Ralph (SPC s r)

The Ralph loop (`SPC s r`) supports two modes based on cursor position:

- **Bead selected** (cursor on a specific bead): sends a targeted prompt with `bd show <id>`, `bd update <id> --status in_progress`, and `bd close <id>` — the agent works on exactly that bead.
- **Resource header** (no bead selected): sends the generic `bd ready` prompt — the agent picks the highest-priority available bead.

Navigation uses a two-level cursor: `j`/`k` move through beads within a resource before advancing to the next resource header.

This replaces the earlier `AgentRunner` interface / `StubRunner` / progress event stream approach for agent execution. The progress/abort infrastructure remains for potential future use but is not the primary agent interaction model.

See `devdeploy-7uj` epic for full details.

### Automated Agent Loop with Rule Injection

**Ralph loop** (`SPC s r`) enables automated development loops by launching an agent with a canned prompt that instructs it to pick work and implement it. For this to work seamlessly, every worktree needs the beads rule and dev-log rule injected automatically, invisible to git.

#### Git-Silent Rule Injection

When devdeploy creates or ensures a worktree (`AddRepo`, `EnsurePRWorktree`), it automatically:

1. Creates `.cursor/rules/` in the worktree with `beads.mdc` and `devdeploy.mdc` (architecture-docs rule)
2. Creates `dev-log/` directory (empty or with a minimal README)
3. Adds these paths to `.git/info/exclude` so git never sees them

`.git/info/exclude` is the ideal mechanism: it's per-worktree, never committed, and works exactly like `.gitignore`. The injected files are ephemeral — they exist only while the worktree lives.

Rule content is stored as embedded Go files (`embed.FS`) in `internal/rules`, making them easy to update without external dependencies.

#### Ralph Loop Execution

When `SPC s r` is pressed on ModeProjectDetail:

1. Checks the selected resource has open beads (already available via beads integration)
2. Creates/ensures worktree (same as `SPC s a`)
3. Ensures rules are injected (idempotent)
4. Splits a tmux pane, runs `agent`, then sends the ralph prompt via `tmux.SendKeys`

The prompt is minimal: "Run `bd ready` to see available work. Pick one issue, claim it with `bd update <id> --status in_progress`, implement it, then close it with `bd close <id>`. Follow the beads and dev-log rules in .cursor/rules/."

#### Idempotency

Rule injection is idempotent — if files already exist with matching content, skip. If `.git/info/exclude` already has the entries, skip. This means `SPC s r` can be pressed repeatedly without duplication.

**Consequences**:
- Worktrees become agent-ready automatically — no manual setup
- Zero git noise: `.git/info/exclude` is invisible to `git status`, `git diff`, etc.
- Dev-log entries created by agents stay local until explicitly committed
- Ralph loop is the simplest possible autonomous agent: one prompt, one resource
- Future: could chain ralph loops across resources, add progress tracking, or add review gates

See `devdeploy-j4n` epic for implementation details.

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
