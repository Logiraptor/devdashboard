package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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
	if cfg.verbose {
		log.Printf("config: workdir=%s project=%q labels=%v max-iterations=%d dry-run=%v",
			cfg.workdir, cfg.project, cfg.labels, cfg.maxIterations, cfg.dryRun)
	}

	// Verify workdir exists.
	info, err := os.Stat(cfg.workdir)
	if err != nil {
		return fmt.Errorf("workdir %q: %w", cfg.workdir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workdir %q is not a directory", cfg.workdir)
	}

	if cfg.dryRun {
		fmt.Println("ralph: dry-run mode, no agents will be executed")
	}

	for i := range cfg.maxIterations {
		if cfg.verbose {
			log.Printf("iteration %d/%d", i+1, cfg.maxIterations)
		}

		// TODO(devdeploy-bkp.2): query bd ready --json for next bead
		// TODO(devdeploy-bkp.3): craft prompt from bead
		// TODO(devdeploy-bkp.4): spawn agent --print --force
		// TODO(devdeploy-bkp.5): assess outcome

		if cfg.dryRun {
			fmt.Printf("ralph: iteration %d/%d — no ready beads (stub)\n", i+1, cfg.maxIterations)
			break // dry-run exits after first pass
		}

		// No real work yet; break to avoid infinite loop in scaffold.
		break
	}

	if cfg.dryRun {
		fmt.Println("ralph: dry-run complete")
	}

	return nil
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "ralph: %v\n", err)
		os.Exit(1)
	}
}
