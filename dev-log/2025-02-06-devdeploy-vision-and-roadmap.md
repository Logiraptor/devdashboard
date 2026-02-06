# DevDeploy: Vision and Long-Term Roadmap

**Date**: 2025-02-06
**Status**: proposed

## Philosophy

DevDeploy exists to **aid engineers in their day-to-day workflow** of designing, writing, reviewing, deploying, and testing code. The workflow increasingly makes use of **agentic workflows**, so managing agent context, plans, and execution is a first-class concern.

## Core Requirements

### Input & Navigation
- **vim + spacemacs keybinds** — familiar modal editing and navigation
- **Leader keys** — SPC as primary leader for command discovery
- **Keybind hints** — after hitting SPC, show available commands as hints (discoverability)

### Agent Workflow UX
- **Live progress windows** — see everything done on your behalf in real time
- **Abort at any time** — ability to cancel/stop agent operations mid-flight

### Technical Foundation
- **Go + Bubble Tea / Charm Bracelet** — TUI framework choice
- **Complex UI** — invest in right abstractions to make modeling easier from the start

## Phased Roadmap

### Phase 1: Foundation
- Initialize Go module with Bubble Tea, Lipgloss, Bubbles
- Establish project structure and dev-log conventions
- Document architecture decisions for UI modeling

### Phase 2: UI Abstractions
- Design abstraction layer for complex TUI composition
- Model: views, panels, focus management, layout
- Consider: view stack, modal overlays, split layouts

### Phase 3: Input System
- Implement leader key (SPC) handling
- vim/spacemacs-style keybind parsing
- Keybind registry with command mapping

### Phase 4: Keybind Hints / Help
- Transient help view triggered after SPC
- Context-aware hints (what's available in current mode/view)
- Dismiss on next key or timeout

### Phase 5: Agent Workflow Integration
- Agent context management (plans, state)
- Integration points for agent execution
- Progress event stream

### Phase 6: Live Progress Windows
- Real-time output display for agent work
- Scrollback, filtering, search
- Integration with abort capability

### Phase 7: Abort & Control
- Cancel signal propagation
- Graceful shutdown of in-flight operations
- User confirmation for destructive aborts

## Dependencies

Phase 1 → Phase 2 → Phase 3 → Phase 4 (foundation and input)
Phase 2 → Phase 5 → Phase 6 → Phase 7 (agent workflow stack)

## Open Questions

- Which Charm libraries beyond bubbletea, lipgloss, bubbles?
- Integration targets: Cursor, Claude Code, custom agents?
- Persistence: where do plans/context live?
