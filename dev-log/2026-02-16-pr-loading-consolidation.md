# PR Loading Consolidation Investigation

**Date**: 2026-02-16  
**Status**: proposed  
**Bead**: devdeploy-fbf.1

## Context

The codebase has multiple methods for loading PRs with overlapping functionality and different return types. This investigation maps all call sites, identifies shared vs unique functionality, and proposes a unified API with an options pattern.

## Current PR Loading Methods

### 1. `CountPRs(projectName string) int`
**Location**: `internal/project/project.go:584`

**Purpose**: Returns total count of open PRs across project repos  
**Implementation**: 
- Sequential `gh pr list --state open` per repo
- Uses `listFilteredPRsInRepo` (author + team review filtering)
- Returns sum of PR counts

**Call Sites**:
- None (deprecated, replaced by `LoadProjectSummary`)

**Requirements**:
- Open PRs only
- Author + team review filtering
- Count only (no PR details)

---

### 2. `LoadProjectSummary(projectName string) DashboardSummary`
**Location**: `internal/project/project.go:616`

**Purpose**: Fetches open PRs once per repo and returns both PR count and resource list  
**Implementation**:
- Parallel PR fetching across repos
- Open PRs only (no merged)
- Returns `DashboardSummary{PRCount, Resources}` where Resources includes repos + open PRs

**Call Sites**:
- `internal/ui/app_commands.go:60` - `enrichesProjectsCmd` (dashboard enrichment)

**Requirements**:
- Open PRs only
- Author + team review filtering
- Parallel fetching across repos
- Returns both count and resource list
- Worktree path detection for existing PR worktrees

---

### 3. `ListProjectPRs(projectName string) ([]RepoPRs, error)`
**Location**: `internal/project/project.go:699`

**Purpose**: Returns PRs grouped by repo (open + recently merged)  
**Implementation**:
- Parallel fetching across repos
- Within each repo: concurrent open + merged PR fetches
- Merged PRs filtered by age (`mergedPRMaxAge = 20 hours`)
- Merged PRs limited (`mergedPRsLimit = 5`)
- Returns `[]RepoPRs` (grouped structure)

**Call Sites**:
- `internal/ui/app_commands.go:112` - `loadProjectPRsCmd` (project detail view)
- `internal/project/project.go:995` - `ListProjectResources` (via internal call)

**Requirements**:
- Open PRs (unlimited)
- Merged PRs (recent, limited to 5, max age 20h)
- Author + team review filtering
- Parallel fetching across repos
- Concurrent open + merged within each repo
- Grouped by repo (`[]RepoPRs`)

---

### 4. `ListProjectResourcesLight(projectName string) []Resource`
**Location**: `internal/project/project.go:948`

**Purpose**: Builds flat resource list from repos + open PRs only  
**Implementation**:
- Parallel open PR fetching across repos
- Returns flat `[]Resource` (repos + PRs interleaved)

**Call Sites**:
- None (appears unused)

**Requirements**:
- Open PRs only
- Author + team review filtering
- Parallel fetching across repos
- Flat resource list (not grouped)
- Worktree path detection

---

### 5. `ListProjectResources(projectName string) []Resource`
**Location**: `internal/project/project.go:993`

**Purpose**: Builds flat resource list from repos + PRs (open + merged)  
**Implementation**:
- Calls `ListProjectPRs` internally (gets open + merged)
- Converts grouped `[]RepoPRs` to flat `[]Resource`

**Call Sites**:
- `internal/ui/app_handlers_project.go:97` - `handleDeleteProject` (for pane cleanup)

**Requirements**:
- Open + merged PRs (via `ListProjectPRs`)
- Flat resource list
- Worktree path detection

---

### 6. `loadProjectPRsCmd(m *Manager, projectName string) tea.Cmd`
**Location**: `internal/ui/app_commands.go:107`

**Purpose**: Async command wrapper for `ListProjectPRs`  
**Call Sites**:
- `internal/ui/app_handlers_project.go:229` - `handleProjectDetailResourcesLoaded`
- `internal/ui/app.go:296` - `newProjectDetailView`

**Requirements**:
- Async execution (goroutine)
- Returns `ProjectPRsLoadedMsg` with grouped PRs

---

### 7. `loadProjectDetailResourcesCmd(m *Manager, projectName string) tea.Cmd`
**Location**: `internal/ui/app_commands.go:83`

**Purpose**: Loads repos instantly (filesystem-only, phase 1)  
**Call Sites**:
- `internal/ui/app_handlers_resource.go:25,47,95` - resource add/remove handlers
- `internal/ui/app_handlers_shell.go:298` - refresh handler

**Requirements**:
- Instant return (filesystem-only)
- Repos only (no PRs)
- Used for progressive loading (phase 1)

---

## Shared Functionality

All methods share:
1. **Author + team review filtering** (`listFilteredPRsInRepo`)
2. **Caching** (45s TTL via `prCache`)
3. **Parallel fetching across repos** (except deprecated `CountPRs`)
4. **Worktree path detection** (checking for existing PR worktrees)
5. **Error handling** (best-effort, continue on errors)

## Unique Functionality

| Method | Unique Aspects |
|--------|---------------|
| `CountPRs` | Count only, sequential (deprecated) |
| `LoadProjectSummary` | Open only, returns count + resources |
| `ListProjectPRs` | Open + merged, grouped by repo, concurrent within repo |
| `ListProjectResourcesLight` | Open only, flat resources (unused) |
| `ListProjectResources` | Open + merged, flat resources |
| `loadProjectPRsCmd` | Async wrapper, returns message |
| `loadProjectDetailResourcesCmd` | Phase 1 only (repos), instant |

## Proposed Unified API

### Core Function: `LoadPRs`

```go
type PRLoadOptions struct {
    // PR states to fetch
    IncludeOpen   bool // default: true
    IncludeMerged bool // default: false
    
    // Merged PR filtering (only applies if IncludeMerged is true)
    MergedLimit   int           // default: 5
    MergedMaxAge  time.Duration // default: 20 hours
    
    // Output format
    Format        PRFormat      // default: FormatGrouped
    
    // Additional options
    IncludeRepos  bool          // default: false (for resource lists)
    CountOnly     bool          // default: false
}

type PRFormat int

const (
    FormatGrouped PRFormat = iota // []RepoPRs
    FormatFlat                     // []Resource
    FormatCount                    // int (requires CountOnly=true)
)

type PRLoadResult struct {
    PRCount   int              // total open PRs (always populated)
    PRsByRepo []RepoPRs        // populated if FormatGrouped
    Resources []Resource       // populated if FormatFlat or IncludeRepos=true
}

func (m *Manager) LoadPRs(projectName string, opts PRLoadOptions) (PRLoadResult, error)
```

### Convenience Methods

```go
// For dashboard: open PRs, count + resources
func (m *Manager) LoadProjectSummary(projectName string) DashboardSummary {
    result, _ := m.LoadPRs(projectName, PRLoadOptions{
        IncludeOpen: true,
        IncludeMerged: false,
        Format: FormatFlat,
        IncludeRepos: true,
    })
    return DashboardSummary{
        PRCount: result.PRCount,
        Resources: result.Resources,
    }
}

// For project detail: open + merged, grouped
func (m *Manager) ListProjectPRs(projectName string) ([]RepoPRs, error) {
    result, err := m.LoadPRs(projectName, PRLoadOptions{
        IncludeOpen: true,
        IncludeMerged: true,
        MergedLimit: mergedPRsLimit,
        MergedMaxAge: mergedPRMaxAge,
        Format: FormatGrouped,
    })
    return result.PRsByRepo, err
}

// For resource lists: open + merged, flat
func (m *Manager) ListProjectResources(projectName string) []Resource {
    result, _ := m.LoadPRs(projectName, PRLoadOptions{
        IncludeOpen: true,
        IncludeMerged: true,
        MergedLimit: mergedPRsLimit,
        MergedMaxAge: mergedPRMaxAge,
        Format: FormatFlat,
        IncludeRepos: true,
    })
    return result.Resources
}
```

## Migration Path

### Phase 1: Add Unified API (Non-Breaking)
1. Add `PRLoadOptions` and `PRLoadResult` types
2. Implement `LoadPRs` with options pattern
3. Keep existing methods as wrappers (backward compatible)

### Phase 2: Migrate Call Sites
1. **Dashboard enrichment** (`enrichesProjectsCmd`):
   - Keep using `LoadProjectSummary` (wrapper)
   - Or migrate to `LoadPRs` directly

2. **Project detail PR loading** (`loadProjectPRsCmd`):
   - Keep using `ListProjectPRs` (wrapper)
   - Or migrate to `LoadPRs` directly

3. **Resource lists** (`ListProjectResources`):
   - Keep as wrapper
   - Or migrate callers to `LoadPRs` directly

4. **Phase 1 repos** (`loadProjectDetailResourcesCmd`):
   - Keep separate (filesystem-only, no PRs)
   - Or add `IncludeRepos: true, IncludeOpen: false` option

### Phase 3: Remove Deprecated Methods
1. Remove `CountPRs` (already unused)
2. Remove `ListProjectResourcesLight` (unused)
3. Consider removing wrappers if all callers migrated

## Benefits

1. **Single source of truth** for PR fetching logic
2. **Flexible options** cover all current use cases
3. **Backward compatible** via wrapper methods
4. **Easier to extend** (new options don't require new methods)
5. **Better testability** (test options independently)
6. **Reduced duplication** (shared parallelization, caching, filtering)

## Open Questions

1. Should `loadProjectDetailResourcesCmd` be unified? (Currently filesystem-only, no PRs)
2. Should async wrappers (`loadProjectPRsCmd`) be part of unified API or stay in UI layer?
3. Should caching be configurable via options? (Currently fixed 45s TTL)
4. Should error handling be configurable? (Currently best-effort)

## Follow-Up Tasks

See created beads for implementation tasks.
