package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"devdeploy/internal/ralph"
	"devdeploy/internal/ralph/tui"
)

// config holds the parsed CLI configuration for a ralph run.
type config struct {
	workdir       string
	bead          string        // bead ID to work on
	maxIterations int           // max agent iterations
	agentTimeout  time.Duration // per-agent timeout
	verbose       bool          // detailed logging
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.workdir, "workdir", "", "path to the repository to operate in (required)")
	flag.StringVar(&cfg.bead, "bead", "", "bead ID to work on (required)")
	flag.IntVar(&cfg.maxIterations, "max-iterations", ralph.DefaultMaxIterations, "maximum agent iterations before giving up")
	flag.DurationVar(&cfg.agentTimeout, "agent-timeout", 10*time.Minute, "per-agent execution timeout")
	flag.BoolVar(&cfg.verbose, "verbose", false, "enable detailed logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ralph --workdir=<path> --bead=<id> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Ralph iteratively runs an agent on a bead until the bead is closed\n")
		fmt.Fprintf(os.Stderr, "or the iteration limit is reached.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExit codes:\n")
		fmt.Fprintf(os.Stderr, "  0  Normal completion (bead closed)\n")
		fmt.Fprintf(os.Stderr, "  1  Runtime error\n")
		fmt.Fprintf(os.Stderr, "  5  Interrupted (SIGINT)\n")
	}

	flag.Parse()

	if cfg.workdir == "" {
		fmt.Fprintln(os.Stderr, "error: --workdir is required")
		flag.Usage()
		os.Exit(1)
	}

	if cfg.bead == "" {
		fmt.Fprintln(os.Stderr, "error: --bead is required")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func run(cfg config) (int, error) {
	// Verify workdir exists
	info, err := os.Stat(cfg.workdir)
	if err != nil {
		return 1, fmt.Errorf("workdir %q: %w", cfg.workdir, err)
	}
	if !info.IsDir() {
		return 1, fmt.Errorf("workdir %q is not a directory", cfg.workdir)
	}

	// Set up context with signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Create tracing observer for OTLP export
	tracingObserver := ralph.NewTracingObserver()

	// Create core with TUI (observer will be set by tui.Run)
	core := &ralph.Core{
		WorkDir:       cfg.workdir,
		RootBead:      cfg.bead,
		MaxIterations: cfg.maxIterations,
		AgentTimeout:  cfg.agentTimeout,
		Output:        io.Discard,
	}

	// Run with TUI, combining tracing observer with TUI observer
	err = tui.Run(ctx, core, tracingObserver)

	// Flush OTLP traces before exit (give 10s for export to complete)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if shutdownErr := tracingObserver.Shutdown(shutdownCtx); shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "ralph: warning: failed to flush traces: %v\n", shutdownErr)
	}

	if err != nil {
		return 1, err
	}

	// Return appropriate exit code based on context cancellation
	if ctx.Err() != nil {
		return 5, nil // Interrupted
	}

	return 0, nil
}

func main() {
	cfg := parseFlags()
	exitCode, err := run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ralph: %v\n", err)
		os.Exit(1)
	}
	os.Exit(exitCode)
}
