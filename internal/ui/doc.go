// Package ui provides the TUI layer for devdeploy using Bubble Tea.
//
// Core abstractions:
//   - View: A screen or major UI region with its own model, update, view (Elm-style)
//   - AppModel: Root model switching between Dashboard and ProjectDetail modes
//   - OverlayStack: Modal/popup views layered on top of the active mode
//   - KeyHandler: Leader-key (SPC) keybind system with mode-aware bindings
//
// See dev-log/ui.md for design rationale.
package ui
