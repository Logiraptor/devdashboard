package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

// DefaultPort is the default trace server port
const DefaultPort = 9876

// Server receives trace events via HTTP
type Server struct {
	manager *Manager
	server  *http.Server
	port    int
}

// NewServer creates a new trace server
// Reads port from DEVDEPLOY_TRACE_PORT env var, defaults to 9876
func NewServer(manager *Manager) *Server {
	port := DefaultPort
	if portStr := os.Getenv("DEVDEPLOY_TRACE_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p < 65536 {
			port = p
		}
	}

	s := &Server{
		manager: manager,
		port:    port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/traces", s.handleTrace)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

// Start begins listening for trace events (non-blocking)
// Returns immediately, server runs in background goroutine
func (s *Server) Start() error {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Trace server error: %v\n", err)
		}
	}()
	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Port returns the port the server is listening on
func (s *Server) Port() int {
	return s.port
}

// handleTrace handles POST /traces requests
func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event TraceEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.manager.HandleEvent(event)
	w.WriteHeader(http.StatusOK)
}
