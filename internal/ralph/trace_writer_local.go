package ralph

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// LocalTraceWriter wraps an io.Writer and emits trace events via LocalTraceEmitter
type LocalTraceWriter struct {
	emitter      *LocalTraceEmitter
	pendingSpans map[string]string // tool call ID -> span ID
	mu           sync.Mutex
	buf          []byte
}

// NewLocalTraceWriter creates a new LocalTraceWriter
func NewLocalTraceWriter(emitter *LocalTraceEmitter) *LocalTraceWriter {
	return &LocalTraceWriter{
		emitter:      emitter,
		pendingSpans: make(map[string]string),
	}
}

// Write implements io.Writer. It parses JSON lines and emits trace events.
func (w *LocalTraceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)

	// Process complete lines
	scanner := bufio.NewScanner(strings.NewReader(string(w.buf)))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		w.processEvent(event)
	}

	// Keep incomplete line in buffer
	if lastNewline := bytes.LastIndex(w.buf, []byte("\n")); lastNewline >= 0 {
		w.buf = w.buf[lastNewline+1:]
	}

	return len(p), nil
}

// processEvent processes a single JSON event and emits trace events if needed.
func (w *LocalTraceWriter) processEvent(event map[string]interface{}) {
	eventType, _ := event["type"].(string)
	if eventType != "tool_call" {
		return
	}

	subtype, _ := event["subtype"].(string)
	toolCallID, _ := event["id"].(string)

	switch subtype {
	case "started":
		toolName, attrs := w.extractToolInfo(event)
		if toolName != "" {
			spanID := w.emitter.StartTool(toolName, attrs)
			if spanID != "" && toolCallID != "" {
				w.pendingSpans[toolCallID] = spanID
			}
		}
	case "completed":
		if toolCallID != "" {
			if spanID, ok := w.pendingSpans[toolCallID]; ok {
				attrs := w.extractResultAttrs(event)
				w.emitter.EndTool(spanID, attrs)
				delete(w.pendingSpans, toolCallID)
			}
		}
	}
}

// extractToolInfo extracts tool name and attributes from a tool_call event.
// Returns tool name and attributes map.
func (w *LocalTraceWriter) extractToolInfo(event map[string]interface{}) (string, map[string]string) {
	attrs := make(map[string]string)

	// Check for new nested schema: {"type":"tool_call","tool_call":{"semSearchToolCall":{...}}}
	if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		return w.extractNestedToolInfo(toolCall, attrs)
	}

	// Fall back to old schema
	name, _ := event["name"].(string)
	if name == "" {
		return "", nil
	}

	args, _ := event["arguments"].(map[string]interface{})
	if args == nil {
		return name, attrs
	}

	// Extract attributes based on tool name
	switch name {
	case "read_file":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "read", attrs
	case "write":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "write", attrs
	case "search_replace":
		if path, ok := args["file_path"].(string); ok {
			attrs["file_path"] = path
		}
		return "edit", attrs
	case "run_terminal_cmd":
		if cmd, ok := args["command"].(string); ok {
			attrs["command"] = cmd
		}
		return "shell", attrs
	case "grep":
		if pattern, ok := args["pattern"].(string); ok {
			attrs["pattern"] = pattern
		}
		return "grep", attrs
	case "codebase_search":
		if query, ok := args["query"].(string); ok {
			attrs["query"] = query
		}
		return "search", attrs
	}

	return name, attrs
}

// extractNestedToolInfo extracts tool info from nested schema.
func (w *LocalTraceWriter) extractNestedToolInfo(toolCall map[string]interface{}, attrs map[string]string) (string, map[string]string) {
	// Check for semantic search
	if sem, ok := toolCall["semSearchToolCall"].(map[string]interface{}); ok {
		if args, ok := sem["args"].(map[string]interface{}); ok {
			if query, ok := args["query"].(string); ok {
				attrs["query"] = query
			}
		}
		return "search", attrs
	}

	// Check for edit
	if edit, ok := toolCall["editToolCall"].(map[string]interface{}); ok {
		if args, ok := edit["args"].(map[string]interface{}); ok {
			if path, ok := args["path"].(string); ok {
				attrs["file_path"] = path
			}
		}
		return "edit", attrs
	}

	// Check for read
	if read, ok := toolCall["readToolCall"].(map[string]interface{}); ok {
		if args, ok := read["args"].(map[string]interface{}); ok {
			if path, ok := args["target_file"].(string); ok {
				attrs["file_path"] = path
			}
		}
		return "read", attrs
	}

	// Check for grep
	if grep, ok := toolCall["grepToolCall"].(map[string]interface{}); ok {
		if args, ok := grep["args"].(map[string]interface{}); ok {
			if pattern, ok := args["pattern"].(string); ok {
				attrs["pattern"] = pattern
			}
		}
		return "grep", attrs
	}

	// Check for shell
	if shell, ok := toolCall["shellToolCall"].(map[string]interface{}); ok {
		if args, ok := shell["args"].(map[string]interface{}); ok {
			if cmd, ok := args["command"].(string); ok {
				attrs["command"] = cmd
			}
		}
		return "shell", attrs
	}

	return "", attrs
}

// extractResultAttrs extracts attributes from a tool_call "completed" event result.
func (w *LocalTraceWriter) extractResultAttrs(event map[string]interface{}) map[string]string {
	attrs := make(map[string]string)

	// Check for result in nested schema
	if toolCall, ok := event["tool_call"].(map[string]interface{}); ok {
		// Check for result in any tool type
		for _, toolData := range toolCall {
			if tool, ok := toolData.(map[string]interface{}); ok {
				if result, ok := tool["result"].(map[string]interface{}); ok {
					// Extract exit code for shell commands
					if exitCode, ok := result["exit_code"].(float64); ok {
						attrs["exit_code"] = fmt.Sprintf("%.0f", exitCode)
					}
					// Extract lines changed for edit operations (if available)
					if linesChanged, ok := result["lines_changed"].(float64); ok {
						attrs["lines_changed"] = fmt.Sprintf("%.0f", linesChanged)
					}
				}
			}
		}
	}

	// Check for result in old schema
	if result, ok := event["result"].(map[string]interface{}); ok {
		if exitCode, ok := result["exit_code"].(float64); ok {
			attrs["exit_code"] = fmt.Sprintf("%.0f", exitCode)
		}
	}

	return attrs
}
