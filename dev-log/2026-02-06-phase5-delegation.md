# Phase 5: Agent Workflow — Delegation Prompts

**Date**: 2026-02-06
**Epic**: devdeploy-i1u.5
**Branch**: `epic-devdeploy-i1u-5`

## Dependency Order

```
5.1 (Artifact store)  ──┐
                        ├──► 5.4 (Wire into app) ──► 5.3 (Agent integration)
5.2 (Progress stream) ──┘
```

- **Wave 1** (parallel): 5.1, 5.2
- **Wave 2**: 5.4 (after 5.1)
- **Wave 3**: 5.3 (after 5.2 and 5.4)

---

## Wave 1 — Parallel Tasks

### Task 5.1 (devdeploy-i1u.5.1): Artifact store and project context

**Goal**: Load and expose plan/design artifacts from project directories.

**Implementation**:
1. Define `~/.devdeploy/projects/<name>/` layout per `dev-log/2026-02-06-ui-abstraction-options.md`:
   - `plan.md`, `design.md` in project root
   - Handle missing files gracefully (empty or "no plan yet" state)
2. Implement `ArtifactStore` (or equivalent) that reads `plan.md`, `design.md` from a project dir
3. Wire `ProjectDetailView` to use real artifact content instead of placeholder
4. Add `internal/ui/artifact.go` or `internal/artifact/store.go`

**Deliverable**: Project detail shows actual plan/design content from disk.

**Reference**: `internal/ui/projectdetail.go` (currently has placeholder `Artifact: "Agent plan (excerpt)..."`)

---

### Task 5.2 (devdeploy-i1u.5.2): Progress event stream (contract)

**Goal**: Define the event contract that Phase 6 will consume for live progress display.

**Implementation**:
1. Define `ProgressEvent` type (message, status, timestamp, optional metadata)
2. Provide a way to emit progress events (channel, `tea.Msg`, or both)
3. Stub/mock emission path so Phase 6 can integrate without blocking on real agent execution

**Deliverable**: Progress event type + emission mechanism; no UI yet (Phase 6).

**Reference**: Add `internal/progress/event.go`

---

## Wave 2 — After 5.1

### Task 5.4 (devdeploy-i1u.5.3): Wire artifact store into app model

**Goal**: App model owns project context and artifact store.

**Implementation**:
1. Add `ArtifactStore` (or `ProjectContext`) to `AppModel`
2. Resolve project dir from project name (config or convention: `~/.devdeploy/projects/<name>/`)
3. Ensure `ProjectDetailView` receives artifact content from app model, not hardcoded

**Deliverable**: Single source of truth for project context; views consume from model.

**Reference**: `internal/ui/app.go`, `internal/ui/projectdetail.go`

---

## Wave 3 — After 5.2 and 5.4

### Task 5.3 (devdeploy-i1u.5.4): Agent execution integration point

**Goal**: Integration point for triggering agent runs and passing context.

**Implementation**:
1. Define `AgentRunner` interface: `Run(ctx, projectDir, planPath, designPath) (tea.Cmd or chan ProgressEvent)`
2. Add SPC keybind `SPC a a` = "agent run" that triggers agent for current project
3. Stub implementation that emits fake progress events (validates the stream)
4. Pass project context (paths) into the runner

**Deliverable**: SPC command triggers agent run; stub emits progress events; Phase 6 can plug in real display.

**Reference**: `internal/ui/keybind.go`, `internal/ui/app.go`, `NewAppModel()` registers keybinds

---

## Multi-Agent Prompt (Wave 1)

Paste into Cursor Multi-Agent (2 parallel agents):

```
Execute these tasks in parallel on branch epic-devdeploy-i1u-5:

Task 1 (devdeploy-i1u.5.1): Artifact store and project context
- Define ~/.devdeploy/projects/<name>/ layout (plan.md, design.md)
- Implement ArtifactStore that reads plan.md, design.md from project dir
- Wire ProjectDetailView to use real artifact content instead of placeholder
- Handle missing files gracefully (empty or "no plan yet")
- Add internal/ui/artifact.go or internal/artifact/store.go
- Commit: feat(artifact): implement ArtifactStore and project layout

Task 2 (devdeploy-i1u.5.2): Progress event stream (contract)
- Define ProgressEvent type (message, status, timestamp, optional metadata)
- Provide emission mechanism (channel, tea.Msg, or both)
- Stub/mock emission path for Phase 6 integration
- Add internal/ui/progress.go or internal/agent/progress.go
- Commit: feat(progress): define ProgressEvent contract and emission mechanism

After both complete, merge to epic-devdeploy-i1u-5.
```

---

## Review Checklist

After all waves complete:

- [x] `ArtifactStore` reads from `~/.devdeploy/projects/<name>/plan.md` and `design.md`
- [x] `ProjectDetailView` shows real content, not placeholder
- [x] `progress.Event` type exists with message, status, timestamp (in `internal/progress`)
- [x] Emission path (tea.Msg + ChanEmitter) usable by Phase 6
- [x] `AppModel` owns `ArtifactStore` and project context
- [x] `SPC a a` triggers agent run (stub)
- [x] Stub agent emits progress events
- [x] No conflicting changes; design coherent per breakdown doc

## Implementation Notes (2026-02-06)

- `ProgressEvent` lives in `internal/progress` as `Event` to avoid ui↔agent import cycle
- Project name normalized: lowercase, spaces→hyphens (e.g. "HA sampler querier" → `ha-sampler-querier/`)
- `DEVDEPLOY_PROJECTS_DIR` env var overrides base path for testing
