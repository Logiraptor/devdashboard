package progress

import "time"

// Status indicates the state of an agent operation.
type Status string

const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusError   Status = "error"
	StatusAborted Status = "aborted"
)

// Event is the contract for live progress display (Phase 6).
// Emitted during agent runs; Phase 6 will consume this for UI.
type Event struct {
	Message   string
	Status    Status
	Timestamp time.Time
	Metadata  map[string]string // optional: step, percent, etc.
}

// ChanEmitter emits events to a channel for Phase 6 to consume.
type ChanEmitter struct {
	Ch chan<- Event
}

// Emit sends the event to the channel (non-blocking; drops if full).
func (e *ChanEmitter) Emit(ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	select {
	case e.Ch <- ev:
	default:
		// Channel full; drop to avoid blocking agent
	}
}
