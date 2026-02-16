package ralph

import (
	"fmt"
	"io"
	"strings"
)

// writef writes formatted output, ignoring errors.
// Use for non-critical output where write failures are acceptable.
func writef(w io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// printVerboseOutput prints verbose agent output (stdout/stderr excerpts).
func printVerboseOutput(out io.Writer, result *AgentResult) {
	if result.Stdout != "" {
		lines := strings.Split(result.Stdout, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			writef(out, "  stdout (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				writef(out, "    %s\n", line)
			}
		} else {
			writef(out, "  stdout:\n")
			for _, line := range lines {
				if line != "" {
					writef(out, "    %s\n", line)
				}
			}
		}
	}
	if result.Stderr != "" {
		lines := strings.Split(result.Stderr, "\n")
		maxLines := 10
		if len(lines) > maxLines {
			writef(out, "  stderr (showing last %d lines):\n", maxLines)
			for _, line := range lines[len(lines)-maxLines:] {
				writef(out, "    %s\n", line)
			}
		} else {
			writef(out, "  stderr:\n")
			for _, line := range lines {
				if line != "" {
					writef(out, "    %s\n", line)
				}
			}
		}
	}
}
