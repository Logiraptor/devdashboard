# Phase 7: Abort/Cancel Capability for In-Flight Operations

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.7

## Context

Phase 6 delivered live progress windows for agent output. Phase 7 adds the ability to abort agent runs mid-flight, so engineers can cancel operations at any time.

## Decision

1. **Cancellable context** — `RunAgentMsg` creates a `context.WithCancel` and stores the cancel func on `AppModel`. The context is passed to `AgentRunner.Run()`.

2. **Runner respects context** — `StubRunner` (and future implementations) check `ctx.Done()` during blocking operations. `emitAfter` uses `select` on `ctx.Done()` vs `time.After(d)`; when cancelled, it emits `StatusAborted` instead of the planned event.

3. **Esc triggers abort** — When the user presses Esc on the ProgressWindow overlay, `DismissModalMsg` is sent. The app checks if the top overlay is ProgressWindow and has an active run; if so, calls the cancel func before popping the overlay.

4. **StatusAborted** — New progress status for aborted runs. ProgressWindow displays it with a ⊗ icon.

5. **Graceful cleanup** — When a run completes (StatusDone or StatusAborted), `agentCancelFunc` is cleared so subsequent Esc just dismisses.

## Consequences

- Engineers can abort agent runs with Esc at any time
- Real agent implementations must check `ctx.Done()` in long-running work
- User confirmation for destructive aborts deferred to future iteration (stub has no side effects)
