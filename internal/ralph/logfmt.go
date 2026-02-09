package ralph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// LogFormatter wraps an io.Writer and formats agent stream-json output
// into human-readable log lines.
type LogFormatter struct {
	w       io.Writer
	verbose bool
	mu      sync.Mutex
	// Counters for summary
	readsCount    int
	writesCount   int
	editsCount    int
	shellsCount   int
	searchesCount int
	thinksCount   int
	errorsCount   int
	// Track last shell command to detect if it had output
	lastShellCmd  string
	shellHadOutput bool
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

		// Format the event (this also updates counters)
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
// Updates internal counters for summary generation.
func (f *LogFormatter) formatEvent(event map[string]interface{}) string {
	f.mu.Lock()
	defer f.mu.Unlock()

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
		f.thinksCount++
		// Skip think events - they're too noisy
		return ""
	case "edit":
		f.editsCount++
		return f.formatEdit(event)
	default:
		// Skip unknown event types
		return ""
	}
}

// formatToolCall formats a tool_call event.
// Only shows significant events (writes, shell commands).
// Counts all events for summary.
func (f *LogFormatter) formatToolCall(event map[string]interface{}) string {
	name, _ := event["name"].(string)
	if name == "" {
		return ""
	}

	// Extract arguments if available
	args, _ := event["arguments"].(map[string]interface{})
	if args == nil {
		return ""
	}

	// Format based on tool name and update counters
	switch name {
	case "read_file":
		f.readsCount++
		// Skip reads - too noisy, will show in summary
		return ""
	case "write":
		f.writesCount++
		if path, ok := args["file_path"].(string); ok {
			contents, _ := args["contents"].(string)
			lines := strings.Count(contents, "\n") + 1
			return fmt.Sprintf("[write] %s (%d lines)", path, lines)
		}
	case "search_replace":
		f.editsCount++
		if path, ok := args["file_path"].(string); ok {
			return fmt.Sprintf("[edit] %s", path)
		}
	case "run_terminal_cmd":
		f.shellsCount++
		if cmd, ok := args["command"].(string); ok {
			// Store command to check if it has output later
			f.lastShellCmd = cmd
			f.shellHadOutput = false
			// Show shell commands - they're significant
			// Truncate long commands
			displayCmd := cmd
			if len(displayCmd) > 60 {
				displayCmd = displayCmd[:57] + "..."
			}
			return fmt.Sprintf("[shell] %s", displayCmd)
		}
	case "grep":
		f.searchesCount++
		// Skip searches - too noisy, will show in summary
		return ""
	case "codebase_search":
		f.searchesCount++
		// Skip searches - too noisy, will show in summary
		return ""
	}

	return ""
}

// formatToolResult formats a tool_result event.
// Shows errors and tracks shell command output.
func (f *LogFormatter) formatToolResult(event map[string]interface{}) string {
	// Track if shell command had output
	if f.lastShellCmd != "" {
		content, _ := event["content"].(string)
		if content != "" && !event["is_error"].(bool) {
			f.shellHadOutput = true
		}
	}

	// Show errors
	if isError, _ := event["is_error"].(bool); isError {
		f.errorsCount++
		content, _ := event["content"].(string)
		if content != "" {
			// Truncate error messages
			if len(content) > 100 {
				content = content[:97] + "..."
			}
			return fmt.Sprintf("[error] %s", content)
		}
	}
	return ""
}

// formatEdit formats an edit event (different from tool_call edit).
func (f *LogFormatter) formatEdit(event map[string]interface{}) string {
	// Edit events might have file paths
	if path, ok := event["file"].(string); ok {
		return fmt.Sprintf("[edit] %s", path)
	}
	return "[edit]"
}

// Summary returns a formatted summary of all events processed.
func (f *LogFormatter) Summary() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	parts := []string{}
	if f.readsCount > 0 {
		parts = append(parts, fmt.Sprintf("read %d", f.readsCount))
	}
	if f.writesCount > 0 {
		parts = append(parts, fmt.Sprintf("wrote %d", f.writesCount))
	}
	if f.editsCount > 0 {
		parts = append(parts, fmt.Sprintf("edited %d", f.editsCount))
	}
	if f.searchesCount > 0 {
		parts = append(parts, fmt.Sprintf("searched %d", f.searchesCount))
	}
	if f.shellsCount > 0 {
		parts = append(parts, fmt.Sprintf("commands %d", f.shellsCount))
	}
	if f.errorsCount > 0 {
		parts = append(parts, fmt.Sprintf("errors %d", f.errorsCount))
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("[ralph] Completed: %s", strings.Join(parts, ", "))
}

// Reset clears all counters (useful for testing or reuse).
func (f *LogFormatter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readsCount = 0
	f.writesCount = 0
	f.editsCount = 0
	f.shellsCount = 0
	f.searchesCount = 0
	f.thinksCount = 0
	f.errorsCount = 0
	f.lastShellCmd = ""
	f.shellHadOutput = false
}
