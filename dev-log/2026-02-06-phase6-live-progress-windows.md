# Phase 6: Live Progress Windows for Agent Output

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.6

## Context

Phase 5 established the progress event stream (`progress.Event`, `ChanEmitter`) and agent runner integration. Phase 6 delivers the live progress display so engineers can see agent work in real time.

## Decision

1. **ProgressWindow view** — A new overlay view that:
   - Displays `progress.Event` stream with timestamps and status icons (● running, ✓ done, ✗ error)
   - Uses `bubbles/viewport` for scrollback (j/k, pgup/pgdown)
   - Shown automatically when user triggers agent run (SPC a a)
   - Dismissed with Esc

2. **StubRunner stream** — Enhanced to emit multiple events over ~2 seconds via `tea.Sequence`, simulating a real agent run for integration testing.

3. **App wiring** — `RunAgentMsg` pushes `ProgressWindow` overlay and runs agent; `progress.Event` messages are forwarded to the overlay for display.

## Consequences

- Engineers see live agent output when running SPC a a from project detail
- Scrollback enables reviewing output after completion
- Filtering and search deferred to future iteration
- Phase 7 (abort) can extend the runner interface; progress window will continue to display events until abort stops the stream
