package claude

// StreamMessage represents a message from Claude's JSON stream output.
//
// Claude CLI with --output-format stream-json produces newline-delimited JSON
// where each line is a StreamMessage. The Type field determines which other
// fields are populated.
//
// Common message types:
//   - "system": System information (session_id, model)
//   - "assistant": Claude's response with content blocks
//   - "result": Final result with cost/duration metrics
//   - "error": Error information
type StreamMessage struct {
	// Type identifies the message kind: "system", "assistant", "result", "error"
	Type string `json:"type"`

	// Subtype provides additional classification within a type
	Subtype string `json:"subtype,omitempty"`

	// SessionID is the unique identifier for this CLI session
	SessionID string `json:"session_id,omitempty"`

	// Model is the Claude model being used (e.g., "claude-sonnet-4-20250514")
	Model string `json:"model,omitempty"`

	// Result contains the final text output for "result" type messages
	Result string `json:"result,omitempty"`

	// TotalCost is the cumulative cost in USD for "result" type messages
	TotalCost float64 `json:"total_cost_usd,omitempty"`

	// DurationMS is the total duration in milliseconds for "result" type messages
	DurationMS int64 `json:"duration_ms,omitempty"`

	// Message contains structured content for "assistant" type messages
	Message *MessageContent `json:"message,omitempty"`

	// Text contains direct text content for some message types
	Text string `json:"text,omitempty"`
}

// MessageContent represents the content of an assistant message.
type MessageContent struct {
	// Role is typically "assistant" for Claude's responses
	Role string `json:"role,omitempty"`

	// Content is the list of content blocks in this message
	Content []ContentBlock `json:"content,omitempty"`
}

// ContentBlock represents a single content block in a message.
//
// Content blocks can be:
//   - Text blocks (Type="text"): Contains Text field
//   - Tool use blocks (Type="tool_use"): Contains Name and Input fields
//   - Tool result blocks (Type="tool_result"): Contains tool execution results
type ContentBlock struct {
	// Type identifies the block kind: "text", "tool_use", "tool_result"
	Type string `json:"type"`

	// Text contains the text content for "text" type blocks
	Text string `json:"text,omitempty"`

	// Name is the tool name for "tool_use" type blocks
	Name string `json:"name,omitempty"`

	// ID is the unique identifier for tool use blocks
	ID string `json:"id,omitempty"`

	// Input contains the tool arguments for "tool_use" type blocks
	Input map[string]any `json:"input,omitempty"`
}

// IsToolUse returns true if this block represents a tool invocation.
func (c *ContentBlock) IsToolUse() bool {
	return c.Type == "tool_use" && c.Name != ""
}

// IsText returns true if this block contains text content.
func (c *ContentBlock) IsText() bool {
	return c.Type == "text"
}

// TodoItem represents a todo item from Claude's TodoWrite tool.
type TodoItem struct {
	// ID is the unique identifier for this todo
	ID string `json:"id,omitempty"`

	// Content is the todo description
	Content string `json:"content"`

	// Status is "pending", "in_progress", or "completed"
	Status string `json:"status"`

	// ActiveForm is the present-tense description shown during execution
	ActiveForm string `json:"activeForm,omitempty"`

	// Priority indicates importance (if specified)
	Priority string `json:"priority,omitempty"`
}
