// Package ralph defines typed structures for parsing tool events from agent output.
package ralph

// toolEventRaw represents the raw JSON structure of a tool_call event.
// It supports both new and legacy formats.
type toolEventRaw struct {
	Type     string                 `json:"type"`
	Subtype  string                 `json:"subtype"`
	CallID   string                 `json:"call_id"`
	Name     string                 `json:"name"`      // Legacy format
	Args     map[string]interface{} `json:"arguments"` // Legacy format
	ToolCall map[string]interface{} `json:"tool_call"` // New format (nil if null or missing)
}

// toolCallNewFormat represents the new agent CLI format where tool_call
// contains typed tool calls like shellToolCall, readToolCall, etc.
type toolCallNewFormat struct {
	ShellToolCall          *toolCallArgs `json:"shellToolCall,omitempty"`
	ReadToolCall           *toolCallArgs `json:"readToolCall,omitempty"`
	WriteToolCall          *toolCallArgs `json:"writeToolCall,omitempty"`
	EditToolCall           *toolCallArgs `json:"editToolCall,omitempty"`
	StrReplaceToolCall     *toolCallArgs `json:"strReplaceToolCall,omitempty"`
	GrepToolCall           *toolCallArgs `json:"grepToolCall,omitempty"`
	GlobToolCall           *toolCallArgs `json:"globToolCall,omitempty"`
	SemanticSearchToolCall *toolCallArgs `json:"semanticSearchToolCall,omitempty"`
	DeleteToolCall         *toolCallArgs `json:"deleteToolCall,omitempty"`
	WebFetchToolCall       *toolCallArgs `json:"webFetchToolCall,omitempty"`
	TodoWriteToolCall      *toolCallArgs `json:"todoWriteToolCall,omitempty"`
}

// toolCallArgs represents the args field within a typed tool call.
type toolCallArgs struct {
	Args map[string]interface{} `json:"args"`
}

// agentResultEventRaw represents the raw JSON structure of a result event.
type agentResultEventRaw struct {
	Type      string                 `json:"type"`
	ChatID    string                 `json:"chatId"`
	ChatIDAlt string                 `json:"chat_id"` // snake_case variant
	Error     interface{}            `json:"error"`   // Can be string or object
	Duration  int                    `json:"duration_ms"`
}

// agentResultError represents an error object in the result event.
type agentResultError struct {
	Message string `json:"message"`
	Code    int    `json:"code,omitempty"`
}
