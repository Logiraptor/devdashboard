package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

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
	workdir       string
	project       string
	labels        stringSlice
	maxIterations int
	dryRun        bool
	verbose       bool
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.workdir, "workdir", "", "path to the worktree to operate in (required)")
	flag.StringVar(&cfg.project, "project", "", "project label filter for bd queries")
	flag.Var(&cfg.labels, "label", "additional label filter (repeatable)")
	flag.IntVar(&cfg.maxIterations, "max-iterations", 20, "safety cap on loop iterations")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "print what would be done without executing agents")
	flag.BoolVar(&cfg.verbose, "verbose", false, "enable detailed logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ralph [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Ralph is an autonomous agent work loop that picks beads from bd\n")
		fmt.Fprintf(os.Stderr, "and dispatches agents to complete them.\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.workdir == "" {
		fmt.Fprintln(os.Stderr, "error: --workdir is required")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func run(cfg config) error {
	// Verify workdir exists before constructing the loop config.
	info, err := os.Stat(cfg.workdir)
	if err != nil {
		return fmt.Errorf("workdir %q: %w", cfg.workdir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workdir %q is not a directory", cfg.workdir)
	}

	// Set up context with signal handling for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	loopCfg := ralph.LoopConfig{
		WorkDir:       cfg.workdir,
		Project:       cfg.project,
		Labels:        cfg.labels,
		MaxIterations: cfg.maxIterations,
		DryRun:        cfg.dryRun,
		Verbose:       cfg.verbose,
	}

	_, err = ralph.Run(ctx, loopCfg)
	return err
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ralph: %v\n", err)
		os.Exit(1)
	}
}
