package claude

// Usage tracks token consumption for a session.
type Usage struct {
	// InputTokens is the total input tokens consumed.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the total output tokens generated.
	OutputTokens int `json:"output_tokens"`

	// CacheCreationInputTokens is tokens used to create prompt cache entries.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`

	// CacheReadInputTokens is tokens read from prompt cache.
	CacheReadInputTokens int `json:"cache_read_input_tokens"`
}

// TotalTokens returns the sum of input and output tokens.
func (u *Usage) TotalTokens() int {
	if u == nil {
		return 0
	}
	return u.InputTokens + u.OutputTokens
}

// StreamMessage represents a message from Claude's JSON stream output.
//
// Claude CLI with --output-format stream-json produces newline-delimited JSON
// where each line is a StreamMessage. The Type field determines which other
// fields are populated.
//
// Message types:
//   - "system" (subtype "init"): Session initialization with tools, model, permissions
//   - "assistant": Claude's response with content blocks (text, tool_use, thinking)
//   - "user": User/tool result messages
//   - "result": Final result with cost/duration/usage metrics
//   - "error": Error information
type StreamMessage struct {
	// Type identifies the message kind: "system", "assistant", "user", "result", "error"
	Type string `json:"type"`

	// Subtype provides additional classification.
	// For "system": "init". For "result": "success", "error_max_turns",
	// "error_during_execution", "error_max_budget_usd", etc.
	Subtype string `json:"subtype,omitempty"`

	// SessionID is the unique identifier for this CLI session.
	SessionID string `json:"session_id,omitempty"`

	// Model is the Claude model being used (e.g., "claude-sonnet-4-20250514").
	Model string `json:"model,omitempty"`

	// UUID is the unique identifier for this specific message (assistant messages).
	UUID string `json:"uuid,omitempty"`

	// ParentToolUseID links subagent responses to their parent tool invocation.
	// nil for top-level messages.
	ParentToolUseID *string `json:"parent_tool_use_id,omitempty"`

	// --- Init message fields (type="system", subtype="init") ---

	// PermissionMode is the active permission mode (from init message).
	PermissionMode string `json:"permissionMode,omitempty"`

	// Tools lists available tool names in the session (from init message).
	Tools []string `json:"tools,omitempty"`

	// --- Content fields ---

	// Message contains structured content for "assistant" and "user" type messages.
	Message *MessageContent `json:"message,omitempty"`

	// Text contains direct text content for some message types.
	Text string `json:"text,omitempty"`

	// --- Result fields (type="result") ---

	// Result contains the final text output.
	Result string `json:"result,omitempty"`

	// CostUSD is the cost for this specific invocation.
	CostUSD float64 `json:"cost_usd,omitempty"`

	// TotalCost is the cumulative cost in USD across the session.
	TotalCost float64 `json:"total_cost_usd,omitempty"`

	// DurationMS is the total duration in milliseconds.
	DurationMS int64 `json:"duration_ms,omitempty"`

	// DurationAPIMS is the API-only duration in milliseconds (excludes tool execution).
	DurationAPIMS int64 `json:"duration_api_ms,omitempty"`

	// IsErrorResult indicates whether the result represents an error.
	IsErrorResult bool `json:"is_error,omitempty"`

	// NumTurns is the number of agentic turns completed.
	NumTurns int `json:"num_turns,omitempty"`

	// Usage contains token consumption data.
	Usage *Usage `json:"usage,omitempty"`

	// StructuredOutput contains validated JSON when --json-schema was used.
	// Type depends on the schema; typically map[string]any after JSON unmarshal.
	StructuredOutput any `json:"structured_output,omitempty"`
}

// MessageContent represents the content of an assistant or user message.
type MessageContent struct {
	// Role is typically "assistant" for Claude's responses or "user" for tool results.
	Role string `json:"role,omitempty"`

	// Content is the list of content blocks in this message.
	Content []ContentBlock `json:"content,omitempty"`
}

// ContentBlock represents a single content block in a message.
//
// Content blocks can be:
//   - Text blocks (Type="text"): Contains Text field
//   - Thinking blocks (Type="thinking"): Contains Thinking field
//   - Tool use blocks (Type="tool_use"): Contains Name, ID, and Input fields
//   - Tool result blocks (Type="tool_result"): Contains ToolUseID and Content fields
type ContentBlock struct {
	// Type identifies the block kind: "text", "thinking", "tool_use", "tool_result"
	Type string `json:"type"`

	// --- Text blocks ---

	// Text contains the text content for "text" type blocks.
	Text string `json:"text,omitempty"`

	// --- Thinking blocks ---

	// Thinking contains Claude's internal reasoning for "thinking" type blocks.
	Thinking string `json:"thinking,omitempty"`

	// --- Tool use blocks ---

	// Name is the tool name for "tool_use" type blocks.
	Name string `json:"name,omitempty"`

	// ID is the unique identifier for tool use blocks, used to match with tool results.
	ID string `json:"id,omitempty"`

	// Input contains the tool arguments for "tool_use" type blocks.
	Input map[string]any `json:"input,omitempty"`

	// --- Tool result blocks ---

	// ToolUseID references the tool_use block this result corresponds to.
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Content contains the tool result content for "tool_result" type blocks.
	// Uses the JSON key "content" which is distinct from the "text" key.
	Content string `json:"content,omitempty"`
}

// IsToolUse returns true if this block represents a tool invocation.
func (c *ContentBlock) IsToolUse() bool {
	return c.Type == "tool_use" && c.Name != ""
}

// IsText returns true if this block contains text content.
func (c *ContentBlock) IsText() bool {
	return c.Type == "text"
}

// IsThinking returns true if this block contains thinking/reasoning content.
func (c *ContentBlock) IsThinking() bool {
	return c.Type == "thinking"
}

// IsToolResult returns true if this block contains a tool execution result.
func (c *ContentBlock) IsToolResult() bool {
	return c.Type == "tool_result"
}

// TodoItem represents a todo item from Claude's TodoWrite tool.
type TodoItem struct {
	// ID is the unique identifier for this todo.
	ID string `json:"id,omitempty"`

	// Content is the todo description.
	Content string `json:"content"`

	// Status is "pending", "in_progress", or "completed".
	Status string `json:"status"`

	// ActiveForm is the present-tense description shown during execution.
	ActiveForm string `json:"activeForm,omitempty"`

	// Priority indicates importance (if specified).
	Priority string `json:"priority,omitempty"`
}
