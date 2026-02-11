package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"devdeploy/internal/ralph"

	tea "github.com/charmbracelet/bubbletea"
)

// config holds the parsed CLI configuration for a ralph run.
type config struct {
	workdir                 string
	epic                    string
	bead                    string // target a specific bead ID
	maxIterations           int
	agentTimeout            time.Duration
	consecutiveFailureLimit int
	timeout                 time.Duration
	concurrency             int // deprecated: use maxParallel instead
	maxParallel             int
	sequential              bool
	dryRun                  bool
	verbose                 bool
	strictLanding           bool
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.workdir, "workdir", "", "path to the worktree to operate in (required)")
	flag.StringVar(&cfg.epic, "epic", "", "epic ID for epic mode: processes leaf tasks sequentially via 'bd ready --parent <epic>', then runs verification pass")
	flag.StringVar(&cfg.bead, "bead", "", "target a specific bead ID (skips picker, sets max-iterations to 1)")
	flag.IntVar(&cfg.maxIterations, "max-iterations", 20, "safety cap on loop iterations")
	flag.DurationVar(&cfg.agentTimeout, "agent-timeout", 10*time.Minute, "per-agent execution timeout")
	flag.IntVar(&cfg.consecutiveFailureLimit, "consecutive-failures", 3, "stop after N consecutive agent failures")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Hour, "total wall-clock timeout for the entire session")
	flag.IntVar(&cfg.concurrency, "concurrency", 0, "DEPRECATED: use --max-parallel instead. number of concurrent agents to run (each uses its own git worktree)")
	flag.IntVar(&cfg.maxParallel, "max-parallel", 1, "maximum number of parallel agents to run (each uses its own git worktree). default is 1 (sequential)")
	flag.BoolVar(&cfg.sequential, "sequential", false, "run agents sequentially (equivalent to --max-parallel=1)")
	flag.BoolVar(&cfg.dryRun, "dry-run", false, "print what would be done without executing agents")
	flag.BoolVar(&cfg.verbose, "verbose", false, "enable detailed logging")
	flag.BoolVar(&cfg.strictLanding, "strict-landing", true, "treat incomplete landing (uncommitted changes or unclosed bead) as failure")

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

	// Handle deprecated --concurrency flag
	if cfg.concurrency > 0 {
		fmt.Fprintln(os.Stderr, "warning: --concurrency is deprecated, use --max-parallel instead")
		if cfg.maxParallel != 1 || cfg.sequential {
			fmt.Fprintln(os.Stderr, "error: cannot use --concurrency together with --max-parallel or --sequential")
			flag.Usage()
			os.Exit(1)
		}
		cfg.maxParallel = cfg.concurrency
	}

	// Handle --sequential flag
	if cfg.sequential {
		if cfg.maxParallel != 1 {
			fmt.Fprintln(os.Stderr, "error: cannot use --sequential together with --max-parallel")
			flag.Usage()
			os.Exit(1)
		}
		cfg.maxParallel = 1
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

	// When --bead is set, implicitly set max-iterations to 1
	maxIterations := cfg.maxIterations
	if cfg.bead != "" {
		maxIterations = 1
	}

	// Epic mode requires sequential processing (maxParallel=1)
	if cfg.epic != "" && cfg.maxParallel > 1 {
		return ralph.StopNormal, fmt.Errorf("--epic requires --max-parallel=1 or --sequential (epic mode processes tasks sequentially)")
	}

	loopCfg := ralph.LoopConfig{
		WorkDir:                 cfg.workdir,
		Epic:                    cfg.epic,
		TargetBead:              cfg.bead,
		MaxIterations:           maxIterations,
		AgentTimeout:            cfg.agentTimeout,
		ConsecutiveFailureLimit: cfg.consecutiveFailureLimit,
		Timeout:                 cfg.timeout,
		Concurrency:             cfg.maxParallel,
		DryRun:                  cfg.dryRun,
		Verbose:                 cfg.verbose,
		StrictLanding:           cfg.strictLanding,
	}

	// Verbose mode: use original non-TUI runner for debugging/compatibility
	if cfg.verbose {
		summary, err := ralph.Run(ctx, loopCfg)
		if err != nil {
			return ralph.StopNormal, err
		}
		return summary.StopReason, nil
	}

	// Normal mode: use TUI
	model := ralph.NewTUIModel(loopCfg)
	
	// Pass context for cancellation
	model.SetContext(ctx, stop)

	// Create program
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Set program reference for trace updates and start the loop
	model.SetProgram(p)

	// Run TUI
	finalModel, err := p.Run()
	if err != nil {
		return ralph.StopNormal, fmt.Errorf("TUI error: %w", err)
	}

	// Extract stop reason from final model
	if m, ok := finalModel.(*ralph.TUIModel); ok {
		if m.Err() != nil {
			return ralph.StopNormal, m.Err()
		}
		if summary := m.Summary(); summary != nil {
			return summary.StopReason, nil
		}
	}

	return ralph.StopNormal, nil
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
