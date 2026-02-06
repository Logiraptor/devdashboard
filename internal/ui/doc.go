// Package ui provides abstraction primitives for complex TUI composition with Bubble Tea.
//
// Core abstractions:
//   - View: A screen or major UI region with its own model, update, view (Elm-style)
//   - Panel: A bounded region within a layout that hosts a View
//   - Layout: Arranges panels (split, stack, overlay)
//   - FocusManager: Tracks and rotates focus across panels
//   - ViewStack: Stack-based navigation (push/pop views)
//   - Overlay: Modal or popup views with dismiss key
//
// See dev-log/2026-02-06-ui-abstraction-options.md for design rationale.
package ui
