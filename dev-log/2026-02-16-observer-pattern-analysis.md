# BeadContextObserver Pattern Analysis

**Date**: 2026-02-16
**Status**: accepted

## Context

The `beadContextObserver` wrapper is used to tag `ToolEvent` instances with a `BeadID` before forwarding them to the inner observer. This enables correct routing of tool events in parallel execution scenarios where multiple agents run concurrently.

## Current Implementation

### Flow
1. `core.go:511` - Wraps observer with `beadContextObserver` before passing to `RunAgent`
2. `executor.go:82-85` - Creates `toolEventWriter` with the wrapped observer
3. `executor.go:385-390` - `toolEventWriter` parses tool events and calls `observer.OnToolStart/OnToolEnd`
4. `core.go:119-127` - `beadContextObserver` sets `event.BeadID = o.beadID` before forwarding to inner observer

### Code Location
- **Wrapper**: `internal/ralph/core.go:85-127`
- **Usage**: `internal/ralph/core.go:511`
- **Consumer**: `internal/ralph/tui/tui.go:270-272` (routes events by BeadID)

## Alternative Approaches Considered

### Option 1: Set BeadID in toolEventWriter
**Approach**: Add `beadID` field to `toolEventWriter` and set it on events before calling observer.

**Pros**:
- Eliminates wrapper layer
- Sets BeadID closer to event creation

**Cons**:
- Requires threading `beadID` through `RunAgent` → `runAgentInternal` → `toolEventWriter`
- Breaks encapsulation: `RunAgent` becomes bead-aware (currently generic)
- Requires changing `RunAgent` signature or adding new option
- `runAgentInternal` is shared by `RunAgent` and `RunAgentOpus` - both would need bead awareness

### Option 2: Set BeadID at ParseToolEvent
**Approach**: Pass `beadID` to `ParseToolEvent` and set it during parsing.

**Pros**:
- Sets BeadID at event creation time

**Cons**:
- `ParseToolEvent` is a pure parsing function - adding context breaks separation of concerns
- `ParseToolEvent` is called from multiple places (tests, other code) that don't have bead context
- Would require making `beadID` optional/nullable, complicating the API

### Option 3: Set BeadID in core.executeBead before calling RunAgent
**Approach**: Create a custom observer wrapper in `executeBead` that sets BeadID.

**Pros**:
- Keeps `RunAgent` generic

**Cons**:
- This is essentially what `beadContextObserver` already does
- Would duplicate the wrapper logic

## Decision

**Keep the `beadContextObserver` wrapper pattern.**

### Rationale

1. **Separation of Concerns**: `RunAgent` remains generic and doesn't need bead awareness. The wrapper encapsulates bead context at the observer boundary.

2. **Standard Pattern**: The decorator/wrapper pattern is a well-established design pattern for adding context to observer chains without modifying core execution logic.

3. **Minimal Coupling**: The wrapper only affects observer notifications, not the agent execution path. This keeps concerns separated.

4. **Flexibility**: The pattern allows different observers to be wrapped with different contexts without modifying `RunAgent` or `toolEventWriter`.

5. **Testability**: The wrapper is easily testable in isolation (see `core_test.go:414-448`).

## Consequences

### Positive
- Clean separation between agent execution and bead context
- `RunAgent` remains reusable for non-bead scenarios
- Easy to add additional context fields in the future if needed
- Pattern is familiar and maintainable

### Tradeoffs
- Adds one layer of indirection (wrapper)
- Requires understanding the wrapper pattern to trace event flow
- Small performance overhead (negligible - just field assignment)

## Conclusion

The `beadContextObserver` wrapper is **necessary and appropriate**. It provides a clean way to inject bead context into tool events without coupling the agent execution layer to bead concepts. The alternative approaches would either break encapsulation or duplicate the wrapper logic.

The wrapper pattern is the right choice here because:
- It maintains clean architecture boundaries
- It's a standard, well-understood pattern
- It's easily testable and maintainable
- The overhead is minimal

**Recommendation**: Keep the current implementation.
