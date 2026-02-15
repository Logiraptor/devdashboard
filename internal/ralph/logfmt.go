// Package ralph implements the autonomous agent work loop.
// logfmt.go provides minimal filtering of agent JSON output.
package ralph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// LogFormatter filters agent stream-json output to show only key events.
// Implements io.Writer for use with io.MultiWriter.
//
// Supports the Cursor CLI stream-json format documented at:
// https://cursor.com/docs/cli/reference/output-format#json-format
type LogFormatter struct {
	w       io.Writer
	verbose bool
}

// NewLogFormatter creates a formatter that writes to w.
// If verbose is true, all output passes through unfiltered.
func NewLogFormatter(w io.Writer, verbose bool) *LogFormatter {
	return &LogFormatter{w: w, verbose: verbose}
}

// Write filters JSON lines, showing only edits and shell commands.
func (f *LogFormatter) Write(p []byte) (int, error) {
	if f.verbose {
		return f.w.Write(p)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		if msg := f.format(event); msg != "" {
			fmt.Fprintln(f.w, msg)
		}
	}
	return len(p), nil
}

// format extracts a one-line summary from a tool_call event.
// Schema: {"type":"tool_call","subtype":"started","tool_call":{...}}
func (f *LogFormatter) format(event map[string]interface{}) string {
	if event["type"] != "tool_call" || event["subtype"] != "started" {
		return ""
	}

	tc, ok := event["tool_call"].(map[string]interface{})
	if !ok {
		return ""
	}

	// writeToolCall: file writes
	if write, ok := tc["writeToolCall"].(map[string]interface{}); ok {
		if args, ok := write["args"].(map[string]interface{}); ok {
			if path, ok := args["path"].(string); ok {
				return fmt.Sprintf("[write] %s", filepath.Base(path))
			}
		}
	}

	// editToolCall: file edits
	if edit, ok := tc["editToolCall"].(map[string]interface{}); ok {
		if args, ok := edit["args"].(map[string]interface{}); ok {
			if path, ok := args["path"].(string); ok {
				return fmt.Sprintf("[edit] %s", filepath.Base(path))
			}
		}
	}

	// shellToolCall: shell commands
	if shell, ok := tc["shellToolCall"].(map[string]interface{}); ok {
		if args, ok := shell["args"].(map[string]interface{}); ok {
			if cmd, ok := args["command"].(string); ok {
				if len(cmd) > 60 {
					cmd = cmd[:57] + "..."
				}
				return fmt.Sprintf("[shell] %s", cmd)
			}
		}
	}

	// tool_call.function: fallback for other tools (per docs)
	if fn, ok := tc["function"].(map[string]interface{}); ok {
		name, _ := fn["name"].(string)
		if name == "" {
			return ""
		}
		argsStr, _ := fn["arguments"].(string)
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			return ""
		}

		switch name {
		case "str_replace_editor", "write":
			if path, ok := args["path"].(string); ok {
				return fmt.Sprintf("[edit] %s", filepath.Base(path))
			}
		case "bash", "shell":
			if cmd, ok := args["command"].(string); ok {
				if len(cmd) > 60 {
					cmd = cmd[:57] + "..."
				}
				return fmt.Sprintf("[shell] %s", cmd)
			}
		}
	}

	return ""
}
