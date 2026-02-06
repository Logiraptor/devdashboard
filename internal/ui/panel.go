package ui

// BoundsFunc returns the panel's position and size given terminal dimensions.
// Returns x, y, width, height.
type BoundsFunc func(width, height int) (x, y, w, h int)

// Panel hosts a View and knows its bounds within a layout.
type Panel struct {
	ID     string
	View   View
	Bounds BoundsFunc
}
