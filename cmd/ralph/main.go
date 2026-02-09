package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"devdeploy/internal/ralph"
)

// stringSlice implements flag.Value for repeatable string flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// config holds the parsed CLI configuration for a ralph run.
type config struct {
	workdir                 string
	project                 string
	epic                    string
	labels                  stringSlice
	maxIterations           int
	agentTimeout            time.Duration
	consecutiveFailureLimit int
	timeout                 time.Duration
	dryRun                  bool
	verbose                 bool
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.workdir, "workdir", "", "path to the worktree to operate in (required)")
	flag.StringVar(&cfg.project, "project", "", "project label filter for bd queries")
	flag.StringVar(&cfg.epic, "epic", "", "epic filter for bd queries (filters to children of the epic)")
	flag.Var(&cfg.labels, "label", "additional label filter (repeatable)")
	flag.IntVar(&cfg.maxIterations, "max-iterations", 20, "safety cap on loop iterations")
	flag.DurationVar(&cfg.agentTimeout, "agent-timeout", 10*time.Minute, "per-agent execution timeout")
	flag.IntVar(&cfg.consecutiveFailureLimit, "consecutive-failures", 3, "stop after N consecutive agent failures")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "total wall-clock timeout for the entire session")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "print what would be done without executing agents")
	flag.BoolVar(&cfg.verbose, "verbose", false, "enable detailed logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ralph [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Ralph is an autonomous agent work loop that picks beads from bd\n")
		fmt.Fprintf(os.Stderr, "and dispatches agents to complete them.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExit codes:\n")
		fmt.Fprintf(os.Stderr, "  0  Normal completion (no more beads)\n")
		fmt.Fprintf(os.Stderr, "  1  Runtime error\n")
		fmt.Fprintf(os.Stderr, "  2  Max iterations reached\n")
		fmt.Fprintf(os.Stderr, "  3  Consecutive failures limit\n")
		fmt.Fprintf(os.Stderr, "  4  Wall-clock timeout\n")
		fmt.Fprintf(os.Stderr, "  5  Interrupted (SIGINT)\n")
		fmt.Fprintf(os.Stderr, "  6  All beads skipped (retry detection)\n")
	}

	flag.Parse()

	if cfg.workdir == "" {
		fmt.Fprintln(os.Stderr, "error: --workdir is required")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func run(cfg config) (ralph.StopReason, error) {
	// Verify workdir exists before constructing the loop config.
	info, err := os.Stat(cfg.workdir)
	if err != nil {
		return ralph.StopNormal, fmt.Errorf("workdir %q: %w", cfg.workdir, err)
	}
	if !info.IsDir() {
		return ralph.StopNormal, fmt.Errorf("workdir %q is not a directory", cfg.workdir)
	}

	// Set up context with signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	loopCfg := ralph.LoopConfig{
		WorkDir:                 cfg.workdir,
		Project:                 cfg.project,
		Epic:                    cfg.epic,
		Labels:                  cfg.labels,
		MaxIterations:           cfg.maxIterations,
		AgentTimeout:            cfg.agentTimeout,
		ConsecutiveFailureLimit: cfg.consecutiveFailureLimit,
		Timeout:                 cfg.timeout,
		DryRun:                  cfg.dryRun,
		Verbose:                 cfg.verbose,
	}

	summary, err := ralph.Run(ctx, loopCfg)
	if err != nil {
		return ralph.StopNormal, err
	}
	return summary.StopReason, nil
}

func main() {
	cfg := parseFlags()
	stopReason, err := run(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ralph: %v\n", err)
		os.Exit(1)
	}
	os.Exit(stopReason.ExitCode())
}
