// Package textutil provides unicode-aware text utilities for TUI rendering.
package textutil

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// TruncateEllipsis is the unicode ellipsis character used for truncation.
const TruncateEllipsis = "…"

// VisualWidth returns the visual width of a string, accounting for unicode characters.
// This is the number of terminal columns the string will occupy.
func VisualWidth(s string) int {
	return runewidth.StringWidth(s)
}

// VisualWidthStyled returns the visual width of a styled string.
// This accounts for ANSI escape codes and unicode characters.
func VisualWidthStyled(s string) int {
	return lipgloss.Width(s)
}

// Truncate truncates a string to fit within maxWidth visual columns.
// If truncation is needed, it appends the unicode ellipsis character (…).
// The result will be at most maxWidth visual columns wide.
func Truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}

	currentWidth := VisualWidth(s)
	if currentWidth <= maxWidth {
		return s
	}

	// We need to truncate. The ellipsis takes 1 column.
	availableWidth := maxWidth - VisualWidth(TruncateEllipsis)
	if availableWidth < 0 {
		return TruncateEllipsis
	}

	// Truncate character by character until we fit
	runes := []rune(s)
	result := make([]rune, 0, len(runes))
	currentResultWidth := 0

	for _, r := range runes {
		runeWidth := runewidth.RuneWidth(r)
		if currentResultWidth+runeWidth > availableWidth {
			break
		}
		result = append(result, r)
		currentResultWidth += runeWidth
	}

	return string(result) + TruncateEllipsis
}

// PadRightVisual pads a string to the right to reach targetWidth visual columns.
// Uses spaces for padding. If the string is already wider than targetWidth, it's truncated.
func PadRightVisual(s string, targetWidth int) string {
	currentWidth := VisualWidth(s)
	if currentWidth >= targetWidth {
		return Truncate(s, targetWidth)
	}
	
	spacesNeeded := targetWidth - currentWidth
	return s + runewidth.FillRight("", spacesNeeded)
}

// PadLeftVisual pads a string to the left to reach targetWidth visual columns.
// Uses spaces for padding. If the string is already wider than targetWidth, it's truncated.
func PadLeftVisual(s string, targetWidth int) string {
	currentWidth := VisualWidth(s)
	if currentWidth >= targetWidth {
		return Truncate(s, targetWidth)
	}
	
	spacesNeeded := targetWidth - currentWidth
	return runewidth.FillLeft("", spacesNeeded) + s
}
