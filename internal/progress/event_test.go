package progress

import (
	"testing"
	"time"
)

func TestStatus_Constants(t *testing.T) {
	if StatusRunning != "running" {
		t.Errorf("StatusRunning: expected 'running', got %q", StatusRunning)
	}
	if StatusDone != "done" {
		t.Errorf("StatusDone: expected 'done', got %q", StatusDone)
	}
	if StatusError != "error" {
		t.Errorf("StatusError: expected 'error', got %q", StatusError)
	}
}

func TestChanEmitter_Emit_SetsTimestampWhenZero(t *testing.T) {
	ch := make(chan Event, 1)
	emitter := &ChanEmitter{Ch: ch}

	ev := Event{Message: "test", Status: StatusRunning}
	emitter.Emit(ev)

	got := <-ch
	if got.Timestamp.IsZero() {
		t.Error("Emit: expected timestamp to be set when zero")
	}
	if got.Message != "test" || got.Status != StatusRunning {
		t.Errorf("Emit: got Message=%q Status=%q", got.Message, got.Status)
	}
}

func TestChanEmitter_Emit_PreservesTimestamp(t *testing.T) {
	ch := make(chan Event, 1)
	emitter := &ChanEmitter{Ch: ch}

	ts := time.Date(2026, 2, 6, 12, 0, 0, 0, time.UTC)
	ev := Event{Message: "test", Status: StatusDone, Timestamp: ts}
	emitter.Emit(ev)

	got := <-ch
	if !got.Timestamp.Equal(ts) {
		t.Errorf("Emit: expected preserved timestamp %v, got %v", ts, got.Timestamp)
	}
}

func TestChanEmitter_Emit_DropsWhenFull(t *testing.T) {
	ch := make(chan Event, 1)
	emitter := &ChanEmitter{Ch: ch}

	// Fill channel
	emitter.Emit(Event{Message: "first"})
	// Second emit should drop (non-blocking)
	emitter.Emit(Event{Message: "dropped"})

	got := <-ch
	if got.Message != "first" {
		t.Errorf("Emit full: expected 'first', got %q", got.Message)
	}
	select {
	case <-ch:
		t.Error("Emit full: expected dropped event not to be sent")
	default:
		// ok
	}
}

func TestChanEmitter_Emit_Metadata(t *testing.T) {
	ch := make(chan Event, 1)
	emitter := &ChanEmitter{Ch: ch}

	ev := Event{
		Message:  "step",
		Status:   StatusRunning,
		Metadata: map[string]string{"step": "2", "percent": "50"},
	}
	emitter.Emit(ev)

	got := <-ch
	if got.Metadata["step"] != "2" || got.Metadata["percent"] != "50" {
		t.Errorf("Emit: expected metadata, got %v", got.Metadata)
	}
}
