package ralph

import (
	"devdeploy/internal/beads"
)

// MultiObserver fans out progress updates to multiple observers.
// It handles nil observers gracefully by skipping them.
type MultiObserver struct {
	observers []ProgressObserver
}

// Ensure MultiObserver implements ProgressObserver.
var _ ProgressObserver = (*MultiObserver)(nil)

// NewMultiObserver creates a MultiObserver that forwards calls to all provided observers.
// Nil observers are filtered out and not included in the list.
func NewMultiObserver(observers ...ProgressObserver) *MultiObserver {
	// Filter out nil observers
	filtered := make([]ProgressObserver, 0, len(observers))
	for _, obs := range observers {
		if obs != nil {
			filtered = append(filtered, obs)
		}
	}
	return &MultiObserver{
		observers: filtered,
	}
}

// safeCall calls fn with panic recovery. One observer failing shouldn't block others.
func safeCall(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			// One observer failing shouldn't block others
		}
	}()
	fn()
}

// OnLoopStart forwards the call to all observers.
func (m *MultiObserver) OnLoopStart(rootBead string) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnLoopStart(rootBead) })
		}
	}
}

// OnBeadStart forwards the call to all observers.
func (m *MultiObserver) OnBeadStart(bead beads.Bead) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnBeadStart(bead) })
		}
	}
}

// OnBeadComplete forwards the call to all observers.
func (m *MultiObserver) OnBeadComplete(result BeadResult) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnBeadComplete(result) })
		}
	}
}

// OnLoopEnd forwards the call to all observers.
func (m *MultiObserver) OnLoopEnd(result *CoreResult) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnLoopEnd(result) })
		}
	}
}

// OnToolStart forwards the call to all observers.
func (m *MultiObserver) OnToolStart(event ToolEvent) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnToolStart(event) })
		}
	}
}

// OnToolEnd forwards the call to all observers.
func (m *MultiObserver) OnToolEnd(event ToolEvent) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnToolEnd(event) })
		}
	}
}

// OnIterationStart forwards the call to all observers.
func (m *MultiObserver) OnIterationStart(iteration int) {
	for _, obs := range m.observers {
		if obs != nil {
			safeCall(func() { obs.OnIterationStart(iteration) })
		}
	}
}
