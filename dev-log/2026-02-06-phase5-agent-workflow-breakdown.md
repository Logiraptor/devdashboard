# Phase 5: Agent Workflow Integration — Breakdown

**Date**: 2026-02-06
**Status**: accepted
**Task**: devdeploy-i1u.5

## Context

Phase 5 from the epic covers three broad areas:
- Agent context management (plans, state)
- Integration points for agent execution
- Progress event stream

This task is large enough to warrant subtasks. The breakdown below defines concrete deliverables that Phase 6 (Live Progress Windows) and Phase 7 (Abort) can build on.

## Subtask Breakdown

Beads IDs: `devdeploy-i1u.5.1` (5.1), `devdeploy-i1u.5.2` (5.2), `devdeploy-i1u.5.3` (5.4), `devdeploy-i1u.5.4` (5.3).

### 5.1 — Artifact store and project context

**Goal**: Load and expose plan/design artifacts from project directories.

- Define `~/.devdeploy/projects/<name>/` layout (per ui-abstraction-options)
- Implement `ArtifactStore` (or equivalent) that reads `plan.md`, `design.md` from a project dir
- Wire `ProjectDetailView` to use real artifact content instead of placeholder
- Handle missing files gracefully (empty or "no plan yet" state)

**Deliverable**: Project detail shows actual plan/design content from disk.

---

### 5.2 — Progress event stream (contract)

**Goal**: Define the event contract that Phase 6 will consume for live progress display.

- Define `ProgressEvent` type (e.g. message, status, timestamp, optional metadata)
- Provide a way to emit progress events (channel, `tea.Msg`, or both)
- Stub/mock emission path so Phase 6 can integrate without blocking on real agent execution

**Deliverable**: Progress event type + emission mechanism; no UI yet (Phase 6).

---

### 5.3 — Agent execution integration point

**Goal**: Integration point for triggering agent runs and passing context.

- Define `AgentRunner` interface (or similar): `Run(ctx, projectDir, planPath, designPath) (tea.Cmd or chan ProgressEvent)`
- Add SPC keybind (e.g. `SPC a a` = "agent run") that triggers agent for current project
- Stub implementation that emits fake progress events (validates the stream)
- Pass project context (paths) into the runner

**Deliverable**: SPC command triggers agent run; stub emits progress events; Phase 6 can plug in real display.

---

### 5.4 — Wire artifact store into app model

**Goal**: App model owns project context and artifact store.

- Add `ArtifactStore` (or `ProjectContext`) to `AppModel`
- Resolve project dir from project name (config or convention)
- Ensure `ProjectDetailView` receives artifact content from app model, not hardcoded

**Deliverable**: Single source of truth for project context; views consume from model.

---

## Dependency Order

```
5.1 (Artifact store)  ──┐
                        ├──► 5.4 (Wire into app) ──► 5.3 (Agent integration)
5.2 (Progress stream) ──┘
```

- 5.1 and 5.2 can be done in parallel
- 5.4 depends on 5.1 (and optionally 5.2 if we want progress in model early)
- 5.3 depends on 5.2 (needs event stream) and 5.4 (needs project context in model)

## Open Questions

- **Integration target**: Cursor, Claude Code, or custom? Start with stub; design interface to be pluggable.
- **Persistence**: Artifacts are files on disk. Do we need additional state (e.g. "last run", "in progress")? Defer to Phase 6/7 if needed.

## Consequences

- Phase 6 can implement live progress windows against the `ProgressEvent` stream
- Phase 7 can add abort by extending the `AgentRunner` interface with cancel
- Real agent integration (Cursor API, etc.) can be added later without changing the core contract
