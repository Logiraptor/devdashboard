package ralph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// LogFormatter wraps an io.Writer and formats agent stream-json output
// into human-readable log lines.
type LogFormatter struct {
	w       io.Writer
	verbose bool
}

// NewLogFormatter creates a new log formatter that writes formatted output to w.
// If verbose is false, only key events are shown. If true, raw JSON is passed through.
func NewLogFormatter(w io.Writer, verbose bool) *LogFormatter {
	return &LogFormatter{
		w:       w,
		verbose: verbose,
	}
}

// Write implements io.Writer. It reads JSON lines, formats them, and writes to the underlying writer.
func (f *LogFormatter) Write(p []byte) (int, error) {
	// In verbose mode, pass through everything
	if f.verbose {
		return f.w.Write(p)
	}

	// Parse line by line
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	var formatted strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Try to parse as JSON
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON, pass through as-is
			fmt.Fprintf(&formatted, "%s\n", line)
			continue
		}

		// Format the event
		formattedLine := f.formatEvent(event)
		if formattedLine != "" {
			fmt.Fprintf(&formatted, "%s\n", formattedLine)
		}
	}

	// Write formatted output
	formattedBytes := []byte(formatted.String())
	n, err := f.w.Write(formattedBytes)
	// Return the original length to satisfy io.Writer contract
	if err == nil && n < len(formattedBytes) {
		// Adjust return value to match original input length
		return len(p), nil
	}
	return len(p), err
}

// formatEvent formats a single JSON event into a human-readable line.
// Returns empty string if the event should be skipped.
func (f *LogFormatter) formatEvent(event map[string]interface{}) string {
	// Extract event type
	eventType, _ := event["type"].(string)
	if eventType == "" {
		return ""
	}

	switch eventType {
	case "tool_call":
		return f.formatToolCall(event)
	case "tool_result":
		return f.formatToolResult(event)
	case "think":
		return f.formatThink(event)
	case "edit":
		return f.formatEdit(event)
	default:
		// Skip unknown event types
		return ""
	}
}

// formatToolCall formats a tool_call event.
func (f *LogFormatter) formatToolCall(event map[string]interface{}) string {
	name, _ := event["name"].(string)
	if name == "" {
		return ""
	}

	// Extract arguments if available
	args, _ := event["arguments"].(map[string]interface{})
	if args == nil {
		return fmt.Sprintf("[tool] %s", name)
	}

	// Format based on tool name
	switch name {
	case "read_file":
		if target, ok := args["target_file"].(string); ok {
			return fmt.Sprintf("[tool] Read: %s", target)
		}
	case "write":
		if path, ok := args["file_path"].(string); ok {
			contents, _ := args["contents"].(string)
			lines := strings.Count(contents, "\n") + 1
			return fmt.Sprintf("[tool] Write: %s (%d lines)", path, lines)
		}
	case "search_replace":
		if path, ok := args["file_path"].(string); ok {
			return fmt.Sprintf("[tool] Edit: %s", path)
		}
	case "run_terminal_cmd":
		if cmd, ok := args["command"].(string); ok {
			// Truncate long commands
			if len(cmd) > 60 {
				cmd = cmd[:57] + "..."
			}
			return fmt.Sprintf("[tool] Shell: %s", cmd)
		}
	case "grep":
		if pattern, ok := args["pattern"].(string); ok {
			// Truncate long patterns
			if len(pattern) > 40 {
				pattern = pattern[:37] + "..."
			}
			return fmt.Sprintf("[tool] Search: %s", pattern)
		}
	case "codebase_search":
		if query, ok := args["query"].(string); ok {
			// Truncate long queries
			if len(query) > 50 {
				query = query[:47] + "..."
			}
			return fmt.Sprintf("[tool] Search: %s", query)
		}
	}

	return fmt.Sprintf("[tool] %s", name)
}

// formatToolResult formats a tool_result event (usually skipped, but could show errors).
func (f *LogFormatter) formatToolResult(event map[string]interface{}) string {
	// Usually skip successful tool results - the tool_call already showed what happened
	// Only show if there's an error
	if isError, _ := event["is_error"].(bool); isError {
		content, _ := event["content"].(string)
		if content != "" {
			// Truncate error messages
			if len(content) > 60 {
				content = content[:57] + "..."
			}
			return fmt.Sprintf("[error] %s", content)
		}
	}
	return ""
}

// formatThink formats a think event.
func (f *LogFormatter) formatThink(event map[string]interface{}) string {
	content, _ := event["content"].(string)
	if content == "" {
		return ""
	}
	// Truncate think content
	if len(content) > 50 {
		content = content[:47] + "..."
	}
	return fmt.Sprintf("[think] %s", content)
}

// formatEdit formats an edit event (different from tool_call edit).
func (f *LogFormatter) formatEdit(event map[string]interface{}) string {
	// Edit events might have file paths
	if path, ok := event["file"].(string); ok {
		return fmt.Sprintf("[edit] %s", path)
	}
	return "[edit]"
}
