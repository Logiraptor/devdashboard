package ralph

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogFormatter wraps an io.Writer and formats agent stream-json output
// into human-readable log lines.
//
// LogFormatter must be created with NewLogFormatter; the zero value is not usable.
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
	errorsCount   int
	// Track last shell command to detect if it had output
	lastShellCmd   string
	shellHadOutput bool
	// Progress display
	beadID       string
	beadTitle    string
	startTime    time.Time
	lastLine     string // Track last output to avoid flicker
	lastNumLines int    // Track number of lines in last display for ANSI clearing
	// Activity stream
	activities    []string // Recent activities
	maxActivities int      // Max to display (default 4)
	// Per-bead tracking
	filesChanged map[string]*FileChange
	testsPassed  int
	testsFailed  int
	failedTests  []string
	lastError    string
}

// Compile-time interface compliance check
var _ io.Writer = (*LogFormatter)(nil)

// FileChange tracks changes to a file.
type FileChange struct {
	Path         string
	LinesAdded   int
	LinesRemoved int
}

// NewLogFormatter creates a new log formatter that writes formatted output to w.
// If verbose is false, only key events are shown. If true, raw JSON is passed through.
func NewLogFormatter(w io.Writer, verbose bool) *LogFormatter {
	return &LogFormatter{
		w:             w,
		verbose:       verbose,
		maxActivities: 4,
		activities:    make([]string, 0, 4),
		filesChanged:  make(map[string]*FileChange),
	}
}

// Write implements io.Writer. It reads JSON lines, formats them, and writes to the underlying writer.
func (f *LogFormatter) Write(p []byte) (int, error) {
	if f.w == nil {
		return 0, fmt.Errorf("LogFormatter not initialized: use NewLogFormatter")
	}
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
			// Not JSON, filter it out
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

	// After processing, update progress line with activities
	f.mu.Lock()
	if f.beadID != "" {
		f.updateDisplay()
	}
	f.mu.Unlock()

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
// Handles documented Cursor CLI stream-json event types:
// - system, user, assistant: skipped (we only care about tool activity)
// - tool_call: only process "started" subtype to avoid double-counting
// - result: final event marking completion (skipped)
func (f *LogFormatter) formatEvent(event map[string]interface{}) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Extract event type
	eventType, _ := event["type"].(string)
	if eventType == "" {
		return ""
	}

	switch eventType {
	case "system", "user", "assistant":
		// Skip - we only care about tool activity
		return ""
	case "tool_call":
		// Only process "started" events to avoid double-counting
		// "completed" events contain results but we've already processed the tool call
		subtype, _ := event["subtype"].(string)
		if subtype == "completed" {
			return ""
		}
		return f.formatToolCall(event)
	case "result":
		// Final event - marks completion and contains timing info
		// Could extract duration_ms if needed, but for now just skip
		return ""
	default:
		// Skip unknown event types
		return ""
	}
}

// formatToolCall formats a tool_call event.
// Only shows significant events (writes, shell commands).
// Counts all events for summary.
func (f *LogFormatter) formatToolCall(event map[string]interface{}) string {
	// Check for new schema: {"type":"tool_call","tool_call":{"semSearchToolCall":{...}}}
	if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		return f.formatNestedToolCall(toolCall)
	}

	// Fall back to old schema for backwards compatibility
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
		if f.beadID != "" {
			// Only show reads if no significant activities
			if f.hasSignificantActivities() {
				return "" // Skip if we have edits/shells/errors
			}
			if path, ok := args["file_path"].(string); ok {
				f.addActivity(fmt.Sprintf("read %s", filepath.Base(path)))
			}
			return ""
		}
		// When no bead, skip reads (too noisy) - backward compatibility
		return ""
	case "write":
		f.writesCount++
		if path, ok := args["file_path"].(string); ok {
			contents, _ := args["contents"].(string)
			lines := strings.Count(contents, "\n") + 1
			// Track file change
			if f.beadID != "" {
				change := f.filesChanged[path]
				if change == nil {
					change = &FileChange{Path: path}
					f.filesChanged[path] = change
				}
				change.LinesAdded += lines
			}
			if f.beadID != "" {
				f.addActivity(fmt.Sprintf("write %s (%d lines)", filepath.Base(path), lines))
				return "" // Don't print inline, activity stream handles it
			}
			return fmt.Sprintf("[write] %s (%d lines)", path, lines)
		}
	case "search_replace":
		f.editsCount++
		if path, ok := args["file_path"].(string); ok {
			// Track file change
			if f.beadID != "" {
				change := f.filesChanged[path]
				if change == nil {
					change = &FileChange{Path: path}
					f.filesChanged[path] = change
				}
				oldStr, _ := args["old_string"].(string)
				newStr, _ := args["new_string"].(string)
				oldLines := strings.Count(oldStr, "\n")
				if oldStr != "" && !strings.HasSuffix(oldStr, "\n") {
					oldLines++
				}
				newLines := strings.Count(newStr, "\n")
				if newStr != "" && !strings.HasSuffix(newStr, "\n") {
					newLines++
				}
				change.LinesRemoved += oldLines
				change.LinesAdded += newLines
			}
			if f.beadID != "" {
				f.addActivity(fmt.Sprintf("edit %s", filepath.Base(path)))
				return "" // Don't print inline, activity stream handles it
			}
			return fmt.Sprintf("[edit] %s", path)
		}
	case "run_terminal_cmd":
		f.shellsCount++
		if cmd, ok := args["command"].(string); ok {
			// Store command to check if it has output later
			f.lastShellCmd = cmd
			f.shellHadOutput = false
			if f.beadID != "" {
				// Add to activity stream when bead is set
				displayCmd := cmd
				if len(displayCmd) > 40 {
					displayCmd = displayCmd[:37] + "..."
				}
				f.addActivity(fmt.Sprintf("shell: %s", displayCmd))
				return "" // Don't print inline, activity stream handles it
			}
			// Return formatted string when no bead (backward compatibility)
			displayCmd := cmd
			if len(displayCmd) > 60 {
				displayCmd = displayCmd[:57] + "..."
			}
			return fmt.Sprintf("[shell] %s", displayCmd)
		}
	case "grep":
		f.searchesCount++
		if f.beadID != "" {
			// Only show grep if no significant activities
			if f.hasSignificantActivities() {
				return "" // Skip if we have edits/shells/errors
			}
			if pattern, ok := args["pattern"].(string); ok {
				displayPattern := pattern
				if len(displayPattern) > 40 {
					displayPattern = displayPattern[:37] + "..."
				}
				f.addActivity(fmt.Sprintf("grep \"%s\"", displayPattern))
			}
			return ""
		}
		// When no bead, skip grep (too noisy) - backward compatibility
		return ""
	case "codebase_search":
		f.searchesCount++
		if f.beadID != "" {
			// Only show searches if no significant activities
			if f.hasSignificantActivities() {
				return "" // Skip if we have edits/shells/errors
			}
			if query, ok := args["query"].(string); ok {
				displayQuery := query
				if len(displayQuery) > 40 {
					displayQuery = displayQuery[:37] + "..."
				}
				f.addActivity(fmt.Sprintf("search \"%s\"", displayQuery))
			}
			return ""
		}
		// When no bead, skip searches (too noisy) - backward compatibility
		return ""
	}

	return ""
}

// formatNestedToolCall formats a tool_call event with the new nested schema.
// The tool_call object contains keys like "semSearchToolCall", "editToolCall", etc.
// Each tool type has an "args" object with metadata and a "result" object (which we ignore).
// Tracks activities for the activity stream instead of returning formatted strings.
func (f *LogFormatter) formatNestedToolCall(toolCall map[string]interface{}) string {
	// Check for semantic search tool call
	if sem, ok := toolCall["semSearchToolCall"].(map[string]interface{}); ok {
		f.searchesCount++
		if args, ok := sem["args"].(map[string]interface{}); ok {
			if query, ok := args["query"].(string); ok {
				if f.beadID != "" {
					// Only show searches if no significant activities
					if f.hasSignificantActivities() {
						return "" // Skip if we have edits/shells/errors
					}
					// Truncate query for display
					displayQuery := query
					if len(displayQuery) > 40 {
						displayQuery = displayQuery[:37] + "..."
					}
					f.addActivity(fmt.Sprintf("search \"%s\"", displayQuery))
					return "" // Don't print inline, activity stream handles it
				}
				// Return formatted string when no bead (backward compatibility)
				displayQuery := query
				if len(displayQuery) > 50 {
					displayQuery = displayQuery[:47] + "..."
				}
				return fmt.Sprintf("[search] %s", displayQuery)
			}
		}
		return "" // Skip if can't extract query
	}

	// Check for edit tool call
	if edit, ok := toolCall["editToolCall"].(map[string]interface{}); ok {
		f.editsCount++
		if args, ok := edit["args"].(map[string]interface{}); ok {
			if path, ok := args["path"].(string); ok {
				// Track file change
				if f.beadID != "" {
					change := f.filesChanged[path]
					if change == nil {
						change = &FileChange{Path: path}
						f.filesChanged[path] = change
					}
					// Try to estimate lines from streamContent if available
					if content, ok := args["streamContent"].(string); ok && content != "" {
						change.LinesAdded += strings.Count(content, "\n")
						if !strings.HasSuffix(content, "\n") {
							change.LinesAdded++
						}
					} else if oldStr, ok := args["old_string"].(string); ok {
						// Fall back to old_string/new_string if available
						newStr, _ := args["new_string"].(string)
						oldLines := strings.Count(oldStr, "\n")
						if oldStr != "" && !strings.HasSuffix(oldStr, "\n") {
							oldLines++
						}
						newLines := strings.Count(newStr, "\n")
						if newStr != "" && !strings.HasSuffix(newStr, "\n") {
							newLines++
						}
						change.LinesRemoved += oldLines
						change.LinesAdded += newLines
					} else {
						// No content info, just mark as edited (1 line estimate)
						change.LinesAdded++
					}
				}
				if f.beadID != "" {
					// Add to activity stream when bead is set
					f.addActivity(fmt.Sprintf("edit %s", filepath.Base(path)))
					return "" // Don't print inline, activity stream handles it
				}
				// Return formatted string when no bead (backward compatibility)
				return fmt.Sprintf("[edit] %s", path)
			}
		}
		return ""
	}

	// Check for read tool call
	if read, ok := toolCall["readToolCall"].(map[string]interface{}); ok {
		f.readsCount++
		if f.beadID != "" {
			// Only show reads if no significant activities
			if f.hasSignificantActivities() {
				return "" // Skip if we have edits/shells/errors
			}
			if args, ok := read["args"].(map[string]interface{}); ok {
				if path, ok := args["target_file"].(string); ok {
					f.addActivity(fmt.Sprintf("read %s", filepath.Base(path)))
				}
			}
			return ""
		}
		// When no bead, skip reads (too noisy) - backward compatibility
		return ""
	}

	// Check for grep tool call
	if grep, ok := toolCall["grepToolCall"].(map[string]interface{}); ok {
		f.searchesCount++
		if f.beadID != "" {
			// Only show grep if no significant activities
			if f.hasSignificantActivities() {
				return "" // Skip if we have edits/shells/errors
			}
			if args, ok := grep["args"].(map[string]interface{}); ok {
				if pattern, ok := args["pattern"].(string); ok {
					displayPattern := pattern
					if len(displayPattern) > 40 {
						displayPattern = displayPattern[:37] + "..."
					}
					f.addActivity(fmt.Sprintf("grep \"%s\"", displayPattern))
				}
			}
			return ""
		}
		// When no bead, skip grep (too noisy) - backward compatibility
		return ""
	}

	// Check for shell tool call
	if shell, ok := toolCall["shellToolCall"].(map[string]interface{}); ok {
		f.shellsCount++
		if args, ok := shell["args"].(map[string]interface{}); ok {
			if cmd, ok := args["command"].(string); ok {
				// Store command to check if it has output later
				f.lastShellCmd = cmd
				f.shellHadOutput = false
				if f.beadID != "" {
					// Add to activity stream when bead is set
					displayCmd := cmd
					if len(displayCmd) > 40 {
						displayCmd = displayCmd[:37] + "..."
					}
					f.addActivity(fmt.Sprintf("shell: %s", displayCmd))
					return "" // Don't print inline, activity stream handles it
				}
				// Return formatted string when no bead (backward compatibility)
				displayCmd := cmd
				if len(displayCmd) > 60 {
					displayCmd = displayCmd[:57] + "..."
				}
				return fmt.Sprintf("[shell] %s", displayCmd)
			}
		}
		return ""
	}

	// Unknown tool type - skip it
	return ""
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

// BeadSummary returns a formatted summary of file changes and test results for the current bead.
func (f *LogFormatter) BeadSummary() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var lines []string

	// Files changed
	if len(f.filesChanged) > 0 {
		var fileParts []string
		for _, fc := range f.filesChanged {
			name := filepath.Base(fc.Path)
			if fc.LinesAdded > 0 && fc.LinesRemoved > 0 {
				fileParts = append(fileParts, fmt.Sprintf("%s (+%d, -%d)", name, fc.LinesAdded, fc.LinesRemoved))
			} else if fc.LinesAdded > 0 {
				fileParts = append(fileParts, fmt.Sprintf("%s (+%d)", name, fc.LinesAdded))
			} else if fc.LinesRemoved > 0 {
				fileParts = append(fileParts, fmt.Sprintf("%s (-%d)", name, fc.LinesRemoved))
			} else {
				fileParts = append(fileParts, name)
			}
		}
		// Limit to 5 files
		if len(fileParts) > 5 {
			fileParts = append(fileParts[:4], fmt.Sprintf("... +%d more", len(fileParts)-4))
		}
		lines = append(lines, fmt.Sprintf("      Files: %s", strings.Join(fileParts, ", ")))
	}

	// Test results
	if f.testsPassed > 0 || f.testsFailed > 0 {
		if f.testsFailed > 0 {
			failed := strings.Join(f.failedTests, ", ")
			if len(failed) > 40 {
				failed = failed[:37] + "..."
			}
			lines = append(lines, fmt.Sprintf("      Tests: %d failed (%s)", f.testsFailed, failed))
		} else {
			lines = append(lines, fmt.Sprintf("      Tests: %d passed", f.testsPassed))
		}
	}

	// Last error
	if f.lastError != "" {
		err := f.lastError
		// Extract first line or truncate
		if idx := strings.Index(err, "\n"); idx > 0 {
			err = err[:idx]
		}
		if len(err) > 60 {
			err = err[:57] + "..."
		}
		lines = append(lines, fmt.Sprintf("      Error: %s", err))
	}

	return strings.Join(lines, "\n")
}

// SetCurrentBead sets the current bead being worked on for progress display.
func (f *LogFormatter) SetCurrentBead(id, title string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beadID = id
	f.beadTitle = title
	f.startTime = time.Now()
	f.lastLine = ""                 // Reset last line to force update
	f.lastNumLines = 0              // Reset line count
	f.activities = f.activities[:0] // Clear activities for new bead
	// Reset per-bead tracking
	f.filesChanged = make(map[string]*FileChange)
	f.testsPassed = 0
	f.testsFailed = 0
	f.failedTests = nil
	f.lastError = ""
}

// renderProgressLine renders the progress line showing current bead and activity counts.
// Must be called with mutex held.
func (f *LogFormatter) renderProgressLine() string {
	elapsed := time.Since(f.startTime).Round(time.Second)

	// Truncate title
	title := f.beadTitle
	if len(title) > 30 {
		title = title[:27] + "..."
	}

	parts := []string{fmt.Sprintf("● %s \"%s\"", f.beadID, title)}
	if f.readsCount > 0 {
		parts = append(parts, fmt.Sprintf("read %d", f.readsCount))
	}
	if f.editsCount > 0 {
		parts = append(parts, fmt.Sprintf("edit %d", f.editsCount))
	}
	if f.searchesCount > 0 {
		parts = append(parts, fmt.Sprintf("search %d", f.searchesCount))
	}
	if f.shellsCount > 0 {
		parts = append(parts, fmt.Sprintf("shell %d", f.shellsCount))
	}
	parts = append(parts, fmt.Sprintf("%ds", int(elapsed.Seconds())))

	return strings.Join(parts, " | ")
}

// renderProgressWithActivities renders the progress line with activity stream below it.
// Must be called with mutex held.
func (f *LogFormatter) renderProgressWithActivities() string {
	var lines []string
	lines = append(lines, f.renderProgressLine())

	// Add activity lines with tree-style prefixes
	for i, activity := range f.activities {
		prefix := "  ├─ "
		if i == len(f.activities)-1 {
			prefix = "  └─ "
		}
		lines = append(lines, prefix+activity)
	}

	return strings.Join(lines, "\n")
}

// addActivity adds an activity to the activity stream.
// Must be called with mutex held.
func (f *LogFormatter) addActivity(activity string) {
	f.activities = append(f.activities, activity)
	if len(f.activities) > f.maxActivities {
		f.activities = f.activities[1:] // Remove oldest
	}
}

// hasSignificantActivities returns true if there are edits, shells, or errors.
// Used to determine if we should show searches/reads in activity stream.
// Must be called with mutex held.
func (f *LogFormatter) hasSignificantActivities() bool {
	return f.editsCount > 0 || f.shellsCount > 0 || f.errorsCount > 0
}

// updateDisplay updates the progress display with activities using ANSI clearing.
// Must be called with mutex held.
func (f *LogFormatter) updateDisplay() {
	if f.beadID == "" {
		return
	}

	display := f.renderProgressWithActivities()
	numLines := strings.Count(display, "\n") + 1

	// Move cursor up and clear lines if we've displayed before
	if f.lastNumLines > 0 {
		// Move up to the first line of the previous display
		writef(f.w, "\033[%dA", f.lastNumLines)
		// Clear each line from top to bottom
		for i := 0; i < f.lastNumLines; i++ {
			writef(f.w, "\033[K") // Clear to end of line
			if i < f.lastNumLines-1 {
				writef(f.w, "\n") // Move to next line (except last)
			}
		}
		// Move back up to the start position
		writef(f.w, "\033[%dA", f.lastNumLines)
	}

	// Write the new display
	writef(f.w, "%s", display)
	f.lastNumLines = numLines
	f.lastLine = display // Update lastLine for comparison
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
	f.errorsCount = 0
	f.lastShellCmd = ""
	f.shellHadOutput = false
	f.beadID = ""
	f.beadTitle = ""
	f.lastLine = ""
	f.lastNumLines = 0
	f.activities = f.activities[:0]
	f.filesChanged = make(map[string]*FileChange)
	f.testsPassed = 0
	f.testsFailed = 0
	f.failedTests = nil
	f.lastError = ""
}
