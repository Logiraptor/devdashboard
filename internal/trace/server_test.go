package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestServer_HandleTrace(t *testing.T) {
	manager := NewManager(10)
	server := NewServer(manager)

	// Test POST /traces with valid event
	t.Run("POST valid event", func(t *testing.T) {
		event := TraceEvent{
			TraceID:   "test-trace-1",
			SpanID:    "test-span-1",
			Type:      EventLoopStart,
			Name:      "test-loop",
			Timestamp: time.Now(),
		}

		body, _ := json.Marshal(event)
		req := httptest.NewRequest(http.MethodPost, "/traces", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleTrace(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		// Verify event was forwarded to manager
		trace := manager.GetTrace("test-trace-1")
		if trace == nil {
			t.Error("Expected trace to be created in manager")
		}
		if trace.ID != "test-trace-1" {
			t.Errorf("Expected trace ID 'test-trace-1', got '%s'", trace.ID)
		}
	})

	// Test POST /traces with invalid JSON
	t.Run("POST invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/traces", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleTrace(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})

	// Test GET /traces returns 405
	t.Run("GET returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/traces", nil)
		w := httptest.NewRecorder()

		server.handleTrace(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", w.Code)
		}
	})
}

func TestServer_StartStop(t *testing.T) {
	manager := NewManager(10)
	server := NewServer(manager)

	// Test server starts
	err := server.Start()
	if err != nil {
		t.Fatalf("Expected Start() to succeed, got error: %v", err)
	}

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify port is correct
	port := server.Port()
	if port != DefaultPort {
		t.Errorf("Expected port %d, got %d", DefaultPort, port)
	}

	// Test server stops cleanly
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	if err != nil {
		t.Errorf("Expected Stop() to succeed, got error: %v", err)
	}
}

func TestServer_CustomPort(t *testing.T) {
	// Set custom port via env var
	os.Setenv("DEVDEPLOY_TRACE_PORT", "9999")
	defer os.Unsetenv("DEVDEPLOY_TRACE_PORT")

	manager := NewManager(10)
	server := NewServer(manager)

	if server.Port() != 9999 {
		t.Errorf("Expected port 9999, got %d", server.Port())
	}
}

func TestServer_InvalidPortEnvVar(t *testing.T) {
	// Set invalid port via env var
	os.Setenv("DEVDEPLOY_TRACE_PORT", "invalid")
	defer os.Unsetenv("DEVDEPLOY_TRACE_PORT")

	manager := NewManager(10)
	server := NewServer(manager)

	// Should fall back to default port
	if server.Port() != DefaultPort {
		t.Errorf("Expected default port %d, got %d", DefaultPort, server.Port())
	}
}

func TestServer_EventsForwardedToManager(t *testing.T) {
	manager := NewManager(10)
	server := NewServer(manager)

	// Test that events are forwarded by calling handleTrace directly
	// (This is tested indirectly in TestServer_HandleTrace, but we verify
	// the forwarding behavior explicitly here)
	event := TraceEvent{
		TraceID:   "forward-trace-1",
		SpanID:    "forward-span-1",
		Type:      EventLoopStart,
		Name:      "forward-loop",
		Timestamp: time.Now(),
	}

	body, _ := json.Marshal(event)
	req := httptest.NewRequest(http.MethodPost, "/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleTrace(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify event was forwarded to manager
	trace := manager.GetTrace("forward-trace-1")
	if trace == nil {
		t.Error("Expected trace to be created in manager")
	}
	if trace.ID != "forward-trace-1" {
		t.Errorf("Expected trace ID 'forward-trace-1', got '%s'", trace.ID)
	}
}
