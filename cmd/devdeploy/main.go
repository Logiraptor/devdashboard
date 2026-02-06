package main

import (
	"fmt"
	"os"

	"devdeploy/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	model := ui.NewAppModel().AsTeaModel()
	p := tea.NewProgram(model)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
