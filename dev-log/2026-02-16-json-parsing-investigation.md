# JSON Parsing Consolidation Investigation

**Date**: 2026-02-16
**Status**: accepted
**Epic**: devdeploy-ptu.1

## Executive Summary

This investigation catalogs all JSON parsing patterns in the codebase, identifies legacy vs current format support requirements, evaluates consolidation opportunities, and assesses test coverage. The codebase uses JSON parsing primarily for:

1. **bd command output** (`bd show`, `bd ready`, `bd list`)
2. **Agent tool event parsing** (streaming JSON from agent CLI)
3. **GitHub PR data** (from `gh pr list`)
4. **Custom enum marshaling** (Outcome, StopReason)

## Catalog of JSON Parsing Functions

### 1. bd Command JSON Parsing

#### 1.1 `bd show --json` (Multiple Variants)

**Locations:**
- `internal/ralph/prompt.go` - `FetchPromptData()` - uses `bdShowFull`
- `internal/ralph/assess.go` - `parseBDShow()` - uses `bdShowEntry`
- `internal/ralph/core.go` - `getBeadIfReady()` - inline struct

**Format Variants:**

**Variant A: `bdShowFull` (prompt.go)**
```go
type bdShowFull struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    Description string `json:"description"`
}
```

**Variant B: `bdShowEntry` (assess.go)**
```go
type bdShowEntry struct {
    ID           string      `json:"id"`
    Status       string      `json:"status"`
    Dependencies []bdShowDep `json:"dependencies"`
    Dependents   []bdShowDep `json:"dependents"`
}
```

**Variant C: Inline struct (core.go)**
```go
struct {
    ID              string    `json:"id"`
    Title           string    `json:"title"`
    Description     string    `json:"description"`
    Status          string    `json:"status"`
    Priority        int       `json:"priority"`
    Labels          []string  `json:"labels"`
    CreatedAt       time.Time `json:"created_at"`
    IssueType       string    `json:"issue_type"`
    DependencyCount int       `json:"dependency_count"`
}
```

**Parsing Pattern:**
- Always returns array (single element)
- Uses `json.Unmarshal()` directly
- Error handling: wraps with context

**Test Coverage:**
- ✅ `internal/ralph/prompt_test.go` - tests `FetchPromptData`
- ✅ `internal/ralph/assess_test.go` - tests `parseBDShow`
- ✅ `internal/ralph/core_test.go` - tests `getBeadIfReady`

**Issues:**
- Three different struct definitions for same command
- Field overlap but different purposes
- No shared base type

#### 1.2 `bd ready --json`

**Location:** `internal/ralph/picker.go` - `parseReadyBeads()`

**Format:**
```go
type bdReadyEntry struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    Status    string    `json:"status"`
    Priority  int       `json:"priority"`
    Labels    []string  `json:"labels"`
    CreatedAt time.Time `json:"created_at"`
}
```

**Parsing Pattern:**
- Returns array
- Uses `json.Unmarshal()` directly
- Maps to `beads.Bead` type

**Test Coverage:**
- ✅ `internal/ralph/picker_test.go` - tests `parseReadyBeads`

**Issues:**
- Similar to `bdShowFull` but different fields
- Could share common fields with other bd structs

#### 1.3 `bd list --json`

**Location:** `internal/beads/beads.go` - `parseBeads()`

**Format:**
```go
type bdListEntry struct {
    ID           string         `json:"id"`
    Title        string         `json:"title"`
    Description  string         `json:"description"`
    Status       string         `json:"status"`
    Priority     int            `json:"priority"`
    Labels       []string       `json:"labels"`
    CreatedAt    time.Time      `json:"created_at"`
    IssueType    string         `json:"issue_type"`
    Dependencies []bdDependency `json:"dependencies"`
}
```

**Parsing Pattern:**
- Returns array
- Uses `json.Unmarshal()` directly
- Maps to `beads.Bead` type

**Test Coverage:**
- ✅ `internal/beads/beads_test.go` - tests `parseBeads`

**Issues:**
- Overlaps significantly with `bdReadyEntry`
- Different dependency structure than `bdShowEntry`

### 2. Agent Tool Event Parsing

#### 2.1 `ParseToolEvent()` - Main Parser

**Location:** `internal/ralph/executor.go`

**Format Support:**

**Legacy Format:**
```json
{
  "type": "tool_call",
  "subtype": "started",
  "name": "read",
  "arguments": {"path": "foo.go"}
}
```

**New Format:**
```json
{
  "type": "tool_call",
  "subtype": "started",
  "call_id": "tool_abc123",
  "tool_call": {
    "shellToolCall": {
      "args": {"command": "ls -la"}
    }
  }
}
```

**Parsing Pattern:**
- Uses `map[string]interface{}` for flexible parsing
- Tries new format first, falls back to legacy
- Handles both "started"/"completed" and "started"/"ended"
- Extracts attributes with type conversion

**Test Coverage:**
- ✅ `internal/ralph/executor_test.go` - extensive tests for both formats
- ✅ `internal/trace/validation_test.go` - tests legacy format parsing

**Issues:**
- Complex dual-format support
- Manual type assertions and conversions
- No schema validation
- Error handling: silently returns nil on parse failure

#### 2.2 `parseAgentResultEvent()` - Result Parser

**Location:** `internal/ralph/executor.go`

**Format:**
```json
{
  "type": "result",
  "chatId": "...",
  "error": "...",
  "duration_ms": 123
}
```

**Parsing Pattern:**
- Scans stdout line-by-line
- Uses `map[string]interface{}`
- Handles both `chatId` and `chat_id` variants
- Handles both string and object error formats

**Test Coverage:**
- ✅ `internal/ralph/executor_test.go` - tests result parsing

**Issues:**
- Manual field extraction
- No structured type
- Handles multiple format variants

### 3. GitHub PR JSON Parsing

**Location:** `internal/project/project.go` - `listPRsInRepo()`

**Format:**
```go
type PRInfo struct {
    Number      int        `json:"number"`
    Title       string     `json:"title"`
    State       string     `json:"state"`
    HeadRefName string     `json:"headRefName"`
    MergedAt    *time.Time `json:"mergedAt"`
}
```

**Parsing Pattern:**
- Uses `gh pr list --json` with field selection
- Direct struct unmarshaling
- Standard Go JSON handling

**Test Coverage:**
- ⚠️ Limited - no dedicated test file found

**Issues:**
- Well-structured, minimal issues
- Could benefit from validation

### 4. Custom Enum Marshaling

#### 4.1 `Outcome` Enum

**Location:** `internal/ralph/assess.go`

**Format:**
- Marshals to string: `"success"`, `"question"`, `"failure"`, `"timeout"`
- Unmarshals from string

**Implementation:**
- Custom `MarshalJSON()` and `UnmarshalJSON()`
- Uses `String()` method for marshaling

**Test Coverage:**
- ✅ `internal/ralph/assess_test.go` - comprehensive tests

**Issues:**
- Well-tested and consistent

#### 4.2 `StopReason` Enum

**Location:** `internal/ralph/loop_types.go`

**Format:**
- Marshals to string: `"normal"`, `"max-iterations"`, etc.
- Unmarshals from string

**Implementation:**
- Custom `MarshalJSON()` and `UnmarshalJSON()`
- Uses `String()` method for marshaling

**Test Coverage:**
- ⚠️ No dedicated tests found (may be tested indirectly)

**Issues:**
- Similar pattern to `Outcome` - could share code

## Legacy vs Current Format Support

### Agent Tool Events

**Legacy Format (Still Supported):**
- Top-level `name` and `arguments` fields
- `subtype: "ended"` instead of `"completed"`
- Simpler structure

**Current Format:**
- Nested `tool_call` object with typed tool calls
- `subtype: "completed"` 
- More structured, type-safe

**Support Strategy:**
- `ParseToolEvent()` supports both formats
- Tries new format first, falls back to legacy
- No deprecation timeline identified

### bd Command Formats

**All formats appear current:**
- `bd show` returns different fields based on context
- `bd ready` and `bd list` have overlapping but distinct purposes
- No legacy format support needed

## Code Generation & Schema-Based Approaches

### Current State

**No code generation:**
- All structs are manually defined
- No JSON schema definitions
- No validation beyond Go's type system

### Opportunities

1. **bd Command Structs:**
   - Could generate from bd command schema (if available)
   - Could create shared base types
   - Could use code generation from OpenAPI/JSON Schema

2. **Agent Tool Events:**
   - Complex format makes code generation challenging
   - Could benefit from schema definition
   - Validation would help catch format changes

3. **GitHub PR Data:**
   - Well-structured, minimal benefit from code generation
   - Could validate against GitHub API schema

### Evaluation

**Code Generation Feasibility:**
- **bd commands:** Medium - would need bd to expose schema
- **Agent events:** Low - format is evolving, dual-format support complicates
- **GitHub PR:** Low - already well-structured

**Schema-Based Approach:**
- Would require external schema definitions
- Benefits: validation, documentation, type safety
- Costs: schema maintenance, generation pipeline

**Recommendation:**
- Start with consolidation of existing structs
- Consider code generation if bd exposes schema
- Focus on shared base types first

## Test Coverage Assessment

### Well-Tested Areas

1. ✅ **bd command parsing:**
   - `parseReadyBeads()` - comprehensive tests
   - `parseBDShow()` - comprehensive tests
   - `FetchPromptData()` - comprehensive tests
   - `parseBeads()` - comprehensive tests

2. ✅ **Agent tool event parsing:**
   - `ParseToolEvent()` - extensive format variation tests
   - Both legacy and new formats tested
   - Edge cases covered

3. ✅ **Enum marshaling:**
   - `Outcome` - comprehensive round-trip tests
   - `StopReason` - indirect coverage

### Gaps

1. ⚠️ **GitHub PR parsing:**
   - No dedicated test file found
   - May be tested indirectly through integration tests

2. ⚠️ **Error handling:**
   - Some parsers silently return nil on error
   - Could benefit from more explicit error testing

3. ⚠️ **Format variation:**
   - Agent result parsing handles multiple variants
   - Could use more explicit variant tests

## Consolidation Opportunities

### High Priority

1. **Consolidate bd show structs:**
   - Create shared base type for common fields
   - Use composition for variant-specific fields
   - Reduces duplication

2. **Unify bd ready/list structs:**
   - Significant overlap between `bdReadyEntry` and `bdListEntry`
   - Could share common fields

### Medium Priority

3. **Extract common parsing utilities:**
   - Shared error handling patterns
   - Common validation helpers
   - Type conversion utilities

4. **Standardize enum marshaling:**
   - Generic helper for string-based enums
   - Reduces duplication between `Outcome` and `StopReason`

### Low Priority

5. **Schema validation:**
   - Add validation for parsed JSON
   - Catch format changes early
   - Improve error messages

## Recommendations

1. **Immediate Actions:**
   - Create shared base types for bd command structs
   - Consolidate `bdShowFull`, `bdShowEntry`, and inline structs
   - Extract common parsing utilities

2. **Follow-up Work:**
   - Evaluate code generation for bd commands (if schema available)
   - Add schema validation for agent events
   - Improve test coverage for GitHub PR parsing

3. **Long-term:**
   - Consider JSON Schema definitions for all formats
   - Evaluate code generation tools (if schemas available)
   - Standardize error handling across parsers

## Follow-up Beads

See child beads created under devdeploy-ptu.1 for specific consolidation tasks.
