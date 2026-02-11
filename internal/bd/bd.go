// Package bd provides utilities for executing bd (beads) commands.
package bd

import "os/exec"

// Runner executes bd commands. The default implementation uses exec.Command.
type Runner func(dir string, args ...string) ([]byte, error)

// Run executes a bd command in the given directory with the provided arguments.
func Run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...)
	cmd.Dir = dir
	return cmd.Output()
}
