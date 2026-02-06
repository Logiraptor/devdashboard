package main

import (
	"fmt"
	"os"

	"devdeploy/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if os.Getenv("TMUX") == "" {
		fmt.Fprintln(os.Stderr, "Run devdeploy inside tmux (e.g. `tmux new -s dev` then `devdeploy`)")
		os.Exit(1)
	}
	model := ui.NewAppModel().AsTeaModel()
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
