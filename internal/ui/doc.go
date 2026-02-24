// Package ui provides the TUI layer for devdeploy using Bubble Tea.
//
// Core abstractions:
//   - View: A screen or major UI region with its own model, update, view (Elm-style)
//   - AppModel: Root model that drives the single Home view
//   - OverlayStack: Modal/popup views layered on top of the active view
//   - KeyHandler: Leader-key (SPC) keybind system
//
// See dev-log/ui.md for design rationale.
package ui
