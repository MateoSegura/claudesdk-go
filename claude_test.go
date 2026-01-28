package claude

import (
	"testing"
)

func TestCLIAvailable(t *testing.T) {
	// This test will pass if Claude CLI is installed, skip otherwise
	if !CLIAvailable() {
		t.Skip("Claude CLI not available")
	}

	version, err := CLIVersion()
	if err != nil {
		t.Errorf("CLIVersion() error: %v", err)
	}
	if version == "" {
		t.Error("CLIVersion() returned empty string")
	}
	t.Logf("Claude CLI version: %s", version)
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		msg  *StreamMessage
		want string
	}{
		{
			name: "nil message",
			msg:  nil,
			want: "",
		},
		{
			name: "direct text field",
			msg:  &StreamMessage{Text: "hello"},
			want: "hello",
		},
		{
			name: "result type",
			msg:  &StreamMessage{Type: "result", Result: "final answer"},
			want: "final answer",
		},
		{
			name: "message content",
			msg: &StreamMessage{
				Message: &MessageContent{
					Content: []ContentBlock{
						{Type: "text", Text: "from content"},
					},
				},
			},
			want: "from content",
		},
		{
			name: "prefers direct text over content",
			msg: &StreamMessage{
				Text: "direct",
				Message: &MessageContent{
					Content: []ContentBlock{
						{Type: "text", Text: "from content"},
					},
				},
			},
			want: "direct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractText(tt.msg)
			if got != tt.want {
				t.Errorf("ExtractText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTodos(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					Input: map[string]any{
						"todos": []any{
							map[string]any{
								"content":    "Write tests",
								"status":     "pending",
								"activeForm": "Writing tests",
							},
							map[string]any{
								"content": "Review code",
								"status":  "completed",
							},
						},
					},
				},
			},
		},
	}

	todos := ExtractTodos(msg)
	if len(todos) != 2 {
		t.Fatalf("ExtractTodos() returned %d todos, want 2", len(todos))
	}

	if todos[0].Content != "Write tests" {
		t.Errorf("todos[0].Content = %q, want %q", todos[0].Content, "Write tests")
	}
	if todos[0].Status != "pending" {
		t.Errorf("todos[0].Status = %q, want %q", todos[0].Status, "pending")
	}
	if todos[0].ActiveForm != "Writing tests" {
		t.Errorf("todos[0].ActiveForm = %q, want %q", todos[0].ActiveForm, "Writing tests")
	}

	if todos[1].Content != "Review code" {
		t.Errorf("todos[1].Content = %q, want %q", todos[1].Content, "Review code")
	}
	if todos[1].Status != "completed" {
		t.Errorf("todos[1].Status = %q, want %q", todos[1].Status, "completed")
	}
}

func TestExtractBashCommand(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{
					Type:  "tool_use",
					Name:  "Bash",
					Input: map[string]any{"command": "ls -la"},
				},
			},
		},
	}

	cmd := ExtractBashCommand(msg)
	if cmd != "ls -la" {
		t.Errorf("ExtractBashCommand() = %q, want %q", cmd, "ls -la")
	}
}

func TestExtractFileAccess(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     string
	}{
		{"Read", "Read", "/path/to/file.go"},
		{"Write", "Write", "/path/to/file.go"},
		{"Edit", "Edit", "/path/to/file.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &StreamMessage{
				Message: &MessageContent{
					Content: []ContentBlock{
						{
							Type:  "tool_use",
							Name:  tt.toolName,
							Input: map[string]any{"file_path": tt.want},
						},
					},
				},
			}

			got := ExtractFileAccess(msg)
			if got != tt.want {
				t.Errorf("ExtractFileAccess() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetToolName(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{Type: "text", Text: "some text"},
				{Type: "tool_use", Name: "Bash"},
			},
		},
	}

	name := GetToolName(msg)
	if name != "Bash" {
		t.Errorf("GetToolName() = %q, want %q", name, "Bash")
	}
}

func TestGetAllToolCalls(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{Type: "text", Text: "let me run some commands"},
				{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "ls"}},
				{Type: "tool_use", Name: "Read", Input: map[string]any{"file_path": "/test"}},
			},
		},
	}

	tools := GetAllToolCalls(msg)
	if len(tools) != 2 {
		t.Fatalf("GetAllToolCalls() returned %d tools, want 2", len(tools))
	}
	if tools[0].Name != "Bash" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "Bash")
	}
	if tools[1].Name != "Read" {
		t.Errorf("tools[1].Name = %q, want %q", tools[1].Name, "Read")
	}
}

func TestIsMessageType(t *testing.T) {
	tests := []struct {
		msg         *StreamMessage
		isResult    bool
		isError     bool
		isAssistant bool
	}{
		{&StreamMessage{Type: "result"}, true, false, false},
		{&StreamMessage{Type: "error"}, false, true, false},
		{&StreamMessage{Type: "assistant"}, false, false, true},
		{&StreamMessage{Type: "system"}, false, false, false},
		{nil, false, false, false},
	}

	for _, tt := range tests {
		if got := IsResult(tt.msg); got != tt.isResult {
			t.Errorf("IsResult(%v) = %v, want %v", tt.msg, got, tt.isResult)
		}
		if got := IsError(tt.msg); got != tt.isError {
			t.Errorf("IsError(%v) = %v, want %v", tt.msg, got, tt.isError)
		}
		if got := IsAssistant(tt.msg); got != tt.isAssistant {
			t.Errorf("IsAssistant(%v) = %v, want %v", tt.msg, got, tt.isAssistant)
		}
	}
}

func TestContentBlockMethods(t *testing.T) {
	textBlock := ContentBlock{Type: "text", Text: "hello"}
	toolBlock := ContentBlock{Type: "tool_use", Name: "Bash"}

	if !textBlock.IsText() {
		t.Error("textBlock.IsText() = false, want true")
	}
	if textBlock.IsToolUse() {
		t.Error("textBlock.IsToolUse() = true, want false")
	}

	if toolBlock.IsText() {
		t.Error("toolBlock.IsText() = true, want false")
	}
	if !toolBlock.IsToolUse() {
		t.Error("toolBlock.IsToolUse() = false, want true")
	}
}

func TestHooksNilSafe(t *testing.T) {
	// Ensure nil hooks don't panic
	var h *Hooks

	// These should not panic
	h.invokeMessage(StreamMessage{})
	h.invokeText("text")
	h.invokeToolCall("tool", nil)
	h.invokeError(nil)
	h.invokeStart(0)
	h.invokeExit(0, 0)

	// Empty hooks struct should also be safe
	h = &Hooks{}
	h.invokeMessage(StreamMessage{})
	h.invokeText("text")
	h.invokeToolCall("tool", nil)
	h.invokeError(nil)
	h.invokeStart(0)
	h.invokeExit(0, 0)
}

func TestErrors(t *testing.T) {
	// Test ParseError
	pe := &ParseError{Line: "invalid json", Err: nil}
	if pe.Error() == "" {
		t.Error("ParseError.Error() returned empty string")
	}

	// Test ExitError
	ee := &ExitError{Code: 1, Stderr: "something went wrong"}
	if ee.Error() == "" {
		t.Error("ExitError.Error() returned empty string")
	}

	// Test StartError
	se := &StartError{Err: ErrCLINotFound}
	if se.Error() == "" {
		t.Error("StartError.Error() returned empty string")
	}
	if se.Unwrap() != ErrCLINotFound {
		t.Error("StartError.Unwrap() didn't return wrapped error")
	}
}

func TestNewLauncher(t *testing.T) {
	l := NewLauncher()
	if l == nil {
		t.Fatal("NewLauncher() returned nil")
	}
	if l.Running() {
		t.Error("NewLauncher().Running() = true, want false")
	}
	if l.PID() != 0 {
		t.Errorf("NewLauncher().PID() = %d, want 0", l.PID())
	}
}

func TestNewSession(t *testing.T) {
	s, err := NewSession(SessionConfig{ID: "test"})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}
	if s == nil {
		t.Fatal("NewSession() returned nil")
	}
	if s.ID != "test" {
		t.Errorf("session.ID = %q, want %q", s.ID, "test")
	}
	if s.Messages == nil {
		t.Error("session.Messages is nil")
	}
	if s.Text == nil {
		t.Error("session.Text is nil")
	}
	if s.Errors == nil {
		t.Error("session.Errors is nil")
	}
}

func TestSessionAutoID(t *testing.T) {
	s, err := NewSession(SessionConfig{})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}
	if s.ID == "" {
		t.Error("session.ID is empty, should be auto-generated")
	}
}
