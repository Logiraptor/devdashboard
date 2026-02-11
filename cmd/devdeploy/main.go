package main

import (
	"context"
	"fmt"
	"os"

	"devdeploy/internal/trace"
	"devdeploy/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if os.Getenv("TMUX") == "" {
		fmt.Fprintln(os.Stderr, "Run devdeploy inside tmux (e.g. `tmux new -s dev` then `devdeploy`)")
		os.Exit(1)
	}

	// Start trace server
	traceMgr := trace.NewManager(10)
	traceServer := trace.NewServer(traceMgr)
	if err := traceServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: trace server failed to start: %v\n", err)
		// Continue anyway - tracing is optional
	}
	defer traceServer.Stop(context.Background())

	// Pass traceMgr to UI model
	model := ui.NewAppModel(ui.WithTraceManager(traceMgr)).AsTeaModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
