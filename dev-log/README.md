# Architecture Decision Log

This directory contains architecture documentation for the devdeploy project.

## Purpose

- Capture the context and reasoning behind major decisions
- Provide a reference for implementation and onboarding
- Explain "why" not just "what"

## Documents

| Document | Contents |
|----------|----------|
| [architecture.md](architecture.md) | Vision, roadmap, tech stack, project structure |
| [ui.md](ui.md) | UI abstractions, layout (Dashboard + Detail), project directory |
| [keybinds.md](keybinds.md) | Keybind system, SPC leader, help view, project/agent keybinds, tmux |
| [agent-workflow.md](agent-workflow.md) | Agent integration, progress, abort, tmux orchestration, validation |
| [2026-02-06-tmux-project-epic.plan.md](2026-02-06-tmux-project-epic.plan.md) | (Superseded) Epic plan: real PRs, selection, SPC s s, tmux organization |
| [2026-02-08-ralph-loop-tool.md](2026-02-08-ralph-loop-tool.md) | Ralph loop tool: dedicated CLI for autonomous agent work loops |
| [2026-02-16-pr-loading-consolidation.md](2026-02-16-pr-loading-consolidation.md) | PR loading consolidation: unified API design with options pattern |
| [2026-02-16-observer-pattern-analysis.md](2026-02-16-observer-pattern-analysis.md) | Observer pattern simplification: analysis and refactoring proposals |
| [2026-02-16-resource-key-typing-investigation.md](2026-02-16-resource-key-typing-investigation.md) | Resource key typing: investigation and design for replacing string keys with typed struct |

## Adding New Decisions

For new architecture decisions, create `YYYY-MM-DD-topic-name.md` and add to this table. Use the format:

```markdown
# Title
**Date**: YYYY-MM-DD
**Status**: proposed | accepted | deprecated

## Context
## Decision
## Consequences
```

Consider consolidating into an existing document when the decision fits a current topic.
