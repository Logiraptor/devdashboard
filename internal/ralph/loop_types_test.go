package ralph

import (
	"encoding/json"
	"testing"
)

func TestStopReason_String(t *testing.T) {
	tests := []struct {
		reason StopReason
		want   string
	}{
		{StopNormal, "normal"},
		{StopMaxIterations, "max-iterations"},
		{StopContextCancelled, "context-cancelled"},
		{StopQuestion, "question"},
		{StopTimeout, "timeout"},
		{StopReason(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.reason.String(); got != tt.want {
			t.Errorf("StopReason(%d).String() = %q, want %q", tt.reason, got, tt.want)
		}
	}
}

func TestStopReason_ExitCode(t *testing.T) {
	tests := []struct {
		reason StopReason
		want   int
	}{
		{StopNormal, 0},
		{StopMaxIterations, 2},
		{StopQuestion, 3},
		{StopTimeout, 4},
		{StopContextCancelled, 5},
		{StopReason(99), 1},
	}
	for _, tt := range tests {
		if got := tt.reason.ExitCode(); got != tt.want {
			t.Errorf("StopReason(%d).ExitCode() = %d, want %d", tt.reason, got, tt.want)
		}
	}
}

func TestMarshalStopReason(t *testing.T) {
	tests := []struct {
		reason StopReason
		want   string
	}{
		{StopNormal, `"normal"`},
		{StopMaxIterations, `"max-iterations"`},
		{StopContextCancelled, `"context-cancelled"`},
		{StopQuestion, `"question"`},
		{StopTimeout, `"timeout"`},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.reason)
		if err != nil {
			t.Errorf("json.Marshal(StopReason(%d)) error = %v", tt.reason, err)
			continue
		}
		if string(got) != tt.want {
			t.Errorf("json.Marshal(StopReason(%d)) = %q, want %q", tt.reason, string(got), tt.want)
		}
	}
}

func TestUnmarshalStopReason(t *testing.T) {
	tests := []struct {
		json string
		want StopReason
	}{
		{`"normal"`, StopNormal},
		{`"max-iterations"`, StopMaxIterations},
		{`"context-cancelled"`, StopContextCancelled},
		{`"question"`, StopQuestion},
		{`"timeout"`, StopTimeout},
	}
	for _, tt := range tests {
		var got StopReason
		err := json.Unmarshal([]byte(tt.json), &got)
		if err != nil {
			t.Errorf("json.Unmarshal(%q, &StopReason) error = %v", tt.json, err)
			continue
		}
		if got != tt.want {
			t.Errorf("json.Unmarshal(%q, &StopReason) = %v, want %v", tt.json, got, tt.want)
		}
	}
}

func TestUnmarshalStopReason_Invalid(t *testing.T) {
	tests := []string{
		`"invalid"`,
		`"unknown"`,
		`123`,
		`null`,
	}
	for _, tt := range tests {
		var got StopReason
		err := json.Unmarshal([]byte(tt), &got)
		if err == nil {
			t.Errorf("json.Unmarshal(%q, &StopReason) expected error, got nil", tt)
		}
	}
}

func TestMarshalUnmarshalStopReason_RoundTrip(t *testing.T) {
	tests := []StopReason{
		StopNormal,
		StopMaxIterations,
		StopContextCancelled,
		StopQuestion,
		StopTimeout,
	}
	for _, want := range tests {
		data, err := json.Marshal(want)
		if err != nil {
			t.Errorf("json.Marshal(%v) error = %v", want, err)
			continue
		}
		var got StopReason
		if err := json.Unmarshal(data, &got); err != nil {
			t.Errorf("json.Unmarshal(%q, &StopReason) error = %v", string(data), err)
			continue
		}
		if got != want {
			t.Errorf("round-trip: got %v, want %v", got, want)
		}
	}
}
