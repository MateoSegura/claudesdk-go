package claude

// ExtractText extracts text content from a StreamMessage.
//
// It checks multiple locations in priority order:
//  1. Direct Text field
//  2. Text blocks in Message.Content
//  3. Result field (for "result" type messages)
//
// Returns empty string if no text content is found.
func ExtractText(msg *StreamMessage) string {
	if msg == nil {
		return ""
	}

	// Direct text field (some message types)
	if msg.Text != "" {
		return msg.Text
	}

	// From message content blocks
	if msg.Message != nil {
		for _, c := range msg.Message.Content {
			if c.Type == "text" && c.Text != "" {
				return c.Text
			}
		}
	}

	// Result field for final output
	if msg.Type == "result" && msg.Result != "" {
		return msg.Result
	}

	return ""
}

// ExtractAllText extracts all text content from a StreamMessage.
//
// Unlike ExtractText which returns the first text found, this returns
// all text blocks concatenated. Useful when a message contains multiple
// text blocks.
func ExtractAllText(msg *StreamMessage) string {
	if msg == nil {
		return ""
	}

	var result string

	// Direct text field
	if msg.Text != "" {
		result += msg.Text
	}

	// All text blocks from message content
	if msg.Message != nil {
		for _, c := range msg.Message.Content {
			if c.Type == "text" && c.Text != "" {
				if result != "" {
					result += "\n"
				}
				result += c.Text
			}
		}
	}

	// Result field
	if msg.Type == "result" && msg.Result != "" {
		if result != "" {
			result += "\n"
		}
		result += msg.Result
	}

	return result
}

// ExtractThinking extracts the first thinking/reasoning content from a message.
//
// Returns empty string if the message doesn't contain thinking blocks.
// Thinking blocks appear when Claude uses extended thinking.
func ExtractThinking(msg *StreamMessage) string {
	if msg == nil || msg.Message == nil {
		return ""
	}

	for _, c := range msg.Message.Content {
		if c.Type == "thinking" && c.Thinking != "" {
			return c.Thinking
		}
	}

	return ""
}

// ExtractAllThinking extracts all thinking content from a message,
// concatenated with newlines.
//
// Returns empty string if no thinking blocks are found.
func ExtractAllThinking(msg *StreamMessage) string {
	if msg == nil || msg.Message == nil {
		return ""
	}

	var result string
	for _, c := range msg.Message.Content {
		if c.Type == "thinking" && c.Thinking != "" {
			if result != "" {
				result += "\n"
			}
			result += c.Thinking
		}
	}

	return result
}

// ExtractTodos extracts todo items from a TodoWrite tool call.
//
// Returns nil if the message doesn't contain a TodoWrite tool call.
func ExtractTodos(msg *StreamMessage) []TodoItem {
	if msg == nil || msg.Message == nil {
		return nil
	}

	for _, c := range msg.Message.Content {
		if c.Type != "tool_use" || c.Name != "TodoWrite" {
			continue
		}
		if c.Input == nil {
			continue
		}

		todosRaw, ok := c.Input["todos"].([]any)
		if !ok {
			continue
		}

		todos := make([]TodoItem, 0, len(todosRaw))
		for _, t := range todosRaw {
			tm, ok := t.(map[string]any)
			if !ok {
				continue
			}
			todos = append(todos, TodoItem{
				ID:         getString(tm, "id"),
				Content:    getString(tm, "content"),
				Status:     getString(tm, "status"),
				ActiveForm: getString(tm, "activeForm"),
				Priority:   getString(tm, "priority"),
			})
		}
		return todos
	}

	return nil
}

// GetToolName returns the name of a tool being invoked, if any.
//
// Returns empty string if the message doesn't contain a tool use block.
func GetToolName(msg *StreamMessage) string {
	if msg == nil || msg.Message == nil {
		return ""
	}

	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && c.Name != "" {
			return c.Name
		}
	}

	return ""
}

// GetToolCall returns the tool name and input from a tool use block.
//
// Returns empty name and nil input if no tool call is present.
func GetToolCall(msg *StreamMessage) (name string, input map[string]any) {
	if msg == nil || msg.Message == nil {
		return "", nil
	}

	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && c.Name != "" {
			return c.Name, c.Input
		}
	}

	return "", nil
}

// GetAllToolCalls returns all tool calls from a message.
//
// A single message may contain multiple tool use blocks when Claude
// decides to invoke multiple tools in parallel.
func GetAllToolCalls(msg *StreamMessage) []ContentBlock {
	if msg == nil || msg.Message == nil {
		return nil
	}

	var tools []ContentBlock
	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && c.Name != "" {
			tools = append(tools, c)
		}
	}
	return tools
}

// ExtractBashCommand extracts the command from a Bash tool call.
//
// Returns empty string if the message doesn't contain a Bash tool call.
func ExtractBashCommand(msg *StreamMessage) string {
	if msg == nil || msg.Message == nil {
		return ""
	}

	for _, c := range msg.Message.Content {
		if c.Type == "tool_use" && c.Name == "Bash" && c.Input != nil {
			if cmd, ok := c.Input["command"].(string); ok {
				return cmd
			}
		}
	}

	return ""
}

// ExtractFileAccess extracts file paths from Read/Write/Edit tool calls.
//
// Returns empty string if the message doesn't contain a file access tool call.
func ExtractFileAccess(msg *StreamMessage) string {
	if msg == nil || msg.Message == nil {
		return ""
	}

	for _, c := range msg.Message.Content {
		if c.Type != "tool_use" || c.Input == nil {
			continue
		}

		switch c.Name {
		case "Read", "Write", "Edit":
			if fp, ok := c.Input["file_path"].(string); ok {
				return fp
			}
		}
	}

	return ""
}

// ExtractAllFileAccess extracts all file paths from a message.
//
// Returns file paths from all Read, Write, Edit, Glob, and Grep tool calls.
func ExtractAllFileAccess(msg *StreamMessage) []string {
	if msg == nil || msg.Message == nil {
		return nil
	}

	var paths []string
	for _, c := range msg.Message.Content {
		if c.Type != "tool_use" || c.Input == nil {
			continue
		}

		switch c.Name {
		case "Read", "Write", "Edit":
			if fp, ok := c.Input["file_path"].(string); ok {
				paths = append(paths, fp)
			}
		case "Glob", "Grep":
			if p, ok := c.Input["path"].(string); ok {
				paths = append(paths, p)
			}
		}
	}

	return paths
}

// ExtractStructuredOutput extracts validated JSON output from a result message.
//
// Returns nil if the message is not a result or has no structured output.
// The returned type depends on the JSON schema used; typically map[string]any.
func ExtractStructuredOutput(msg *StreamMessage) any {
	if msg == nil || msg.Type != "result" {
		return nil
	}
	return msg.StructuredOutput
}

// ExtractUsage extracts token usage data from a result message.
//
// Returns nil if the message is not a result or has no usage data.
func ExtractUsage(msg *StreamMessage) *Usage {
	if msg == nil || msg.Type != "result" {
		return nil
	}
	return msg.Usage
}

// ExtractInitTools extracts the list of available tool names from an init message.
//
// Returns nil if the message is not a system init message.
func ExtractInitTools(msg *StreamMessage) []string {
	if msg == nil || msg.Type != "system" || msg.Subtype != "init" {
		return nil
	}
	return msg.Tools
}

// ExtractInitPermissionMode extracts the permission mode from an init message.
//
// Returns empty string if the message is not a system init message.
func ExtractInitPermissionMode(msg *StreamMessage) string {
	if msg == nil || msg.Type != "system" || msg.Subtype != "init" {
		return ""
	}
	return msg.PermissionMode
}

// --- Message type predicates ---

// IsResult returns true if this is a final result message with metrics.
func IsResult(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "result"
}

// IsError returns true if this is an error message.
func IsError(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "error"
}

// IsAssistant returns true if this is an assistant response message.
func IsAssistant(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "assistant"
}

// IsSystem returns true if this is a system message.
func IsSystem(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "system"
}

// IsInit returns true if this is a system init message (first message in stream).
func IsInit(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "system" && msg.Subtype == "init"
}

// IsUser returns true if this is a user/tool-result message.
func IsUser(msg *StreamMessage) bool {
	return msg != nil && msg.Type == "user"
}

// getString safely extracts a string from a map.
func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
