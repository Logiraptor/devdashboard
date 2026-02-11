package trace

import (
	"encoding/hex"
	"testing"
)

func TestNewTraceID_GeneratesValidHex(t *testing.T) {
	id := NewTraceID()
	if len(id) != 32 {
		t.Errorf("NewTraceID: expected 32 characters, got %d", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("NewTraceID: generated invalid hex: %v", err)
	}
}

func TestNewTraceID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewTraceID()
	id2 := NewTraceID()
	if id1 == id2 {
		t.Error("NewTraceID: generated duplicate IDs")
	}
}

func TestNewSpanID_GeneratesValidHex(t *testing.T) {
	id := NewSpanID()
	if len(id) != 16 {
		t.Errorf("NewSpanID: expected 16 characters, got %d", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("NewSpanID: generated invalid hex: %v", err)
	}
}

func TestNewSpanID_GeneratesUniqueIDs(t *testing.T) {
	id1 := NewSpanID()
	id2 := NewSpanID()
	if id1 == id2 {
		t.Error("NewSpanID: generated duplicate IDs")
	}
}
