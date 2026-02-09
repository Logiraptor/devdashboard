# devdeploy

> **Personal Tool Notice**: This is a personal workflow tool built for my own development process. It's not designed, documented, or supported for use by others. You're welcome to browse the code for ideas, but please don't expect it to work for your setup or use case.

## What is this?

devdeploy is a terminal-based control panel for managing my day-to-day development workflow. It ties together the tools I use constantly—git worktrees, tmux panes, GitHub PRs, and AI coding agents—into a single interface with vim-style keybindings.

The core idea: projects contain resources (repositories, PRs), and the primary actions are opening a shell or launching an agent in a worktree. Everything is orchestrated through tmux.

## Key Components

- **TUI Dashboard**: Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), uses spacemacs-style `SPC` leader key with contextual hints
- **Worktree Management**: Automatically creates and manages git worktrees for PRs and feature branches
- **Tmux Integration**: Splits panes, tracks sessions, runs agents in dedicated panes
- **Ralph**: An autonomous agent loop that picks tasks from my issue tracker (beads/bd) and dispatches AI agents to implement them
- **Rule Injection**: Automatically injects cursor rules into worktrees (git-silent via `.git/info/exclude`)

## Tech Stack

- Go 1.25
- Bubble Tea / Lipgloss / Bubbles (Charm Bracelet)
- tmux (required—won't run without it)
- [beads](https://github.com/beads-project/bd) for issue tracking

## Structure

```
devdeploy/
├── cmd/devdeploy/     # Main TUI entrypoint
├── cmd/ralph/         # Autonomous agent loop CLI
├── internal/          # Private packages (ui, tmux, ralph, beads, etc.)
├── dev-log/           # Architecture decision records
└── contrib/           # tmux configs
```

## Why not use X instead?

This tool exists because I wanted something that fits exactly how I work. It's opinionated, incomplete in many ways, and evolves with my needs. If you're looking for a general-purpose tool, this isn't it.
