package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// CLI availability
// ---------------------------------------------------------------------------

func TestCLIAvailable(t *testing.T) {
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

func TestVersion(t *testing.T) {
	if Version != "0.2.0" {
		t.Errorf("Version = %q, want %q", Version, "0.2.0")
	}
}

// ---------------------------------------------------------------------------
// buildArgs unit tests
// ---------------------------------------------------------------------------

func TestBuildArgsBase(t *testing.T) {
	args, err := buildArgs("hello world", LaunchOptions{}, "")
	if err != nil {
		t.Fatalf("buildArgs() error: %v", err)
	}

	// Always-present flags
	assertContains(t, args, "--print")
	assertContains(t, args, "--output-format")
	assertContains(t, args, "stream-json")
	assertContains(t, args, "--verbose")

	// Prompt must be last
	if args[len(args)-1] != "hello world" {
		t.Errorf("prompt not last arg: got %q", args[len(args)-1])
	}
}

func TestBuildArgsPermissionMode(t *testing.T) {
	tests := []struct {
		name         string
		opts         LaunchOptions
		wantFlag     string
		wantValue    string
		dontWantFlag string
	}{
		{
			name:     "permission mode bypass",
			opts:     LaunchOptions{PermissionMode: PermissionBypass},
			wantFlag: "--permission-mode", wantValue: "bypassPermissions",
		},
		{
			name:     "permission mode plan",
			opts:     LaunchOptions{PermissionMode: PermissionPlan},
			wantFlag: "--permission-mode", wantValue: "plan",
		},
		{
			name:     "permission mode acceptEdits",
			opts:     LaunchOptions{PermissionMode: PermissionAcceptEdits},
			wantFlag: "--permission-mode", wantValue: "acceptEdits",
		},
		{
			name:         "skip permissions fallback",
			opts:         LaunchOptions{SkipPermissions: true},
			wantFlag:     "--dangerously-skip-permissions",
			dontWantFlag: "--permission-mode",
		},
		{
			name:     "permission mode takes precedence over skip",
			opts:     LaunchOptions{PermissionMode: PermissionPlan, SkipPermissions: true},
			wantFlag: "--permission-mode", wantValue: "plan",
			dontWantFlag: "--dangerously-skip-permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := buildArgs("test", tt.opts, "")
			if err != nil {
				t.Fatalf("buildArgs() error: %v", err)
			}

			assertContains(t, args, tt.wantFlag)
			if tt.wantValue != "" {
				assertContainsPair(t, args, tt.wantFlag, tt.wantValue)
			}
			if tt.dontWantFlag != "" {
				assertNotContains(t, args, tt.dontWantFlag)
			}
		})
	}
}

func TestBuildArgsAllowDangerouslySkipPermissions(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{AllowDangerouslySkipPermissions: true}, "")
	assertContains(t, args, "--allow-dangerously-skip-permissions")
}

func TestBuildArgsAllowedTools(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		AllowedTools: []string{"Bash(git log *)", "Read"},
	}, "")

	assertContainsPair(t, args, "--allowedTools", "Bash(git log *)")
	assertContainsPair(t, args, "--allowedTools", "Read")
}

func TestBuildArgsDisallowedTools(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		DisallowedTools: []string{"Edit", "Write"},
	}, "")

	assertContainsPair(t, args, "--disallowedTools", "Edit")
	assertContainsPair(t, args, "--disallowedTools", "Write")
}

func TestBuildArgsPermissionPromptTool(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{PermissionPromptTool: "mcp_auth"}, "")
	assertContainsPair(t, args, "--permission-prompt-tool", "mcp_auth")
}

func TestBuildArgsModel(t *testing.T) {
	tests := []struct {
		name  string
		opts  LaunchOptions
		flag  string
		value string
	}{
		{"model", LaunchOptions{Model: "sonnet"}, "--model", "sonnet"},
		{"fallback model", LaunchOptions{FallbackModel: "haiku"}, "--fallback-model", "haiku"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := buildArgs("test", tt.opts, "")
			assertContainsPair(t, args, tt.flag, tt.value)
		})
	}
}

func TestBuildArgsBudget(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{MaxBudgetUSD: 5.50}, "")
	assertContainsPair(t, args, "--max-budget-usd", "5.50")
}

func TestBuildArgsBetas(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		Betas: []string{"interleaved-thinking", "context-1m"},
	}, "")
	assertContainsPair(t, args, "--betas", "interleaved-thinking,context-1m")
}

func TestBuildArgsSystemPrompt(t *testing.T) {
	tests := []struct {
		name     string
		opts     LaunchOptions
		wantFlag string
		wantVal  string
		dontWant string
	}{
		{
			name:     "system prompt",
			opts:     LaunchOptions{SystemPrompt: "You are a pirate"},
			wantFlag: "--system-prompt", wantVal: "You are a pirate",
		},
		{
			name:     "system prompt file",
			opts:     LaunchOptions{SystemPromptFile: "/tmp/prompt.txt"},
			wantFlag: "--system-prompt-file", wantVal: "/tmp/prompt.txt",
		},
		{
			name:     "system prompt takes precedence over file",
			opts:     LaunchOptions{SystemPrompt: "inline", SystemPromptFile: "/tmp/file.txt"},
			wantFlag: "--system-prompt", wantVal: "inline",
			dontWant: "--system-prompt-file",
		},
		{
			name:     "append system prompt",
			opts:     LaunchOptions{AppendSystemPrompt: "Always end with DONE"},
			wantFlag: "--append-system-prompt", wantVal: "Always end with DONE",
		},
		{
			name:     "append system prompt file",
			opts:     LaunchOptions{AppendSystemPromptFile: "/tmp/append.txt"},
			wantFlag: "--append-system-prompt-file", wantVal: "/tmp/append.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := buildArgs("test", tt.opts, "")
			assertContainsPair(t, args, tt.wantFlag, tt.wantVal)
			if tt.dontWant != "" {
				assertNotContains(t, args, tt.dontWant)
			}
		})
	}
}

func TestBuildArgsSession(t *testing.T) {
	tests := []struct {
		name     string
		opts     LaunchOptions
		wantFlag string
		wantVal  string
	}{
		{"resume", LaunchOptions{Resume: "abc123"}, "--resume", "abc123"},
		{"session-id", LaunchOptions{SessionID: "550e8400-e29b-41d4-a716-446655440000"}, "--session-id", "550e8400-e29b-41d4-a716-446655440000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, _ := buildArgs("test", tt.opts, "")
			assertContainsPair(t, args, tt.wantFlag, tt.wantVal)
		})
	}
}

func TestBuildArgsSessionBoolFlags(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		Continue:             true,
		ForkSession:          true,
		NoSessionPersistence: true,
	}, "")

	assertContains(t, args, "--continue")
	assertContains(t, args, "--fork-session")
	assertContains(t, args, "--no-session-persistence")
}

func TestBuildArgsMaxTurns(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{MaxTurns: 5}, "")
	assertContainsPair(t, args, "--max-turns", "5")
}

func TestBuildArgsTools(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		Tools: []string{"Bash", "Edit", "Read"},
	}, "")
	assertContainsPair(t, args, "--tools", "Bash,Edit,Read")
}

func TestBuildArgsAgents(t *testing.T) {
	agents := map[string]AgentDefinition{
		"reviewer": {
			Description: "Code reviewer",
			Prompt:      "Review code for bugs",
			Tools:       []string{"Read", "Grep"},
			Model:       "sonnet",
		},
	}
	args, err := buildArgs("test", LaunchOptions{Agents: agents}, "")
	if err != nil {
		t.Fatalf("buildArgs() error: %v", err)
	}

	// Find the --agents value and verify it's valid JSON
	idx := indexOfArg(args, "--agents")
	if idx < 0 || idx+1 >= len(args) {
		t.Fatal("--agents flag not found")
	}

	var parsed map[string]AgentDefinition
	if err := json.Unmarshal([]byte(args[idx+1]), &parsed); err != nil {
		t.Fatalf("agents JSON parse error: %v", err)
	}
	if parsed["reviewer"].Description != "Code reviewer" {
		t.Errorf("agent description = %q, want %q", parsed["reviewer"].Description, "Code reviewer")
	}
	if len(parsed["reviewer"].Tools) != 2 {
		t.Errorf("agent tools length = %d, want 2", len(parsed["reviewer"].Tools))
	}
}

func TestBuildArgsDisableSlashCommands(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{DisableSlashCommands: true}, "")
	assertContains(t, args, "--disable-slash-commands")
}

func TestBuildArgsJSONSchema(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
		"required": []any{"answer"},
	}
	args, err := buildArgs("test", LaunchOptions{JSONSchema: schema}, "")
	if err != nil {
		t.Fatalf("buildArgs() error: %v", err)
	}

	idx := indexOfArg(args, "--json-schema")
	if idx < 0 || idx+1 >= len(args) {
		t.Fatal("--json-schema flag not found")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(args[idx+1]), &parsed); err != nil {
		t.Fatalf("schema JSON parse error: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema type = %v, want object", parsed["type"])
	}
}

func TestBuildArgsIncludePartialMessages(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{IncludePartialMessages: true}, "")
	assertContains(t, args, "--include-partial-messages")
}

func TestBuildArgsInputFormat(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{InputFormat: "stream-json"}, "")
	assertContainsPair(t, args, "--input-format", "stream-json")
}

func TestBuildArgsSettings(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		SettingSources: []string{"user", "project"},
		Settings:       "/tmp/settings.json",
	}, "")

	assertContainsPair(t, args, "--setting-sources", "user,project")
	assertContainsPair(t, args, "--settings", "/tmp/settings.json")
}

func TestBuildArgsPluginDirs(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		PluginDirs: []string{"/plugins/a", "/plugins/b"},
	}, "")

	assertContainsPair(t, args, "--plugin-dir", "/plugins/a")
	assertContainsPair(t, args, "--plugin-dir", "/plugins/b")
}

func TestBuildArgsAddDirs(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		AddDirs: []string{"../apps", "../lib"},
	}, "")

	assertContainsPair(t, args, "--add-dir", "../apps")
	assertContainsPair(t, args, "--add-dir", "../lib")
}

func TestBuildArgsMCPConfig(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{}, "/tmp/mcp.json")
	assertContainsPair(t, args, "--mcp-config", "/tmp/mcp.json")
}

func TestBuildArgsStrictMCP(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{StrictMCP: true}, "")
	assertContains(t, args, "--strict-mcp-config")
}

func TestBuildArgsDebug(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{Debug: "api,mcp"}, "")
	assertContainsPair(t, args, "--debug", "api,mcp")
}

func TestBuildArgsChrome(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		args, _ := buildArgs("test", LaunchOptions{Chrome: BoolPtr(true)}, "")
		assertContains(t, args, "--chrome")
		assertNotContains(t, args, "--no-chrome")
	})

	t.Run("disable", func(t *testing.T) {
		args, _ := buildArgs("test", LaunchOptions{Chrome: BoolPtr(false)}, "")
		assertContains(t, args, "--no-chrome")
	})

	t.Run("nil (default)", func(t *testing.T) {
		args, _ := buildArgs("test", LaunchOptions{}, "")
		assertNotContains(t, args, "--chrome")
		assertNotContains(t, args, "--no-chrome")
	})
}

func TestBuildArgsAdditionalArgs(t *testing.T) {
	args, _ := buildArgs("test", LaunchOptions{
		AdditionalArgs: []string{"--custom-flag", "value"},
	}, "")

	assertContains(t, args, "--custom-flag")
	assertContains(t, args, "value")
}

func TestBuildArgsComprehensive(t *testing.T) {
	// Test a fully-loaded options struct
	opts := LaunchOptions{
		PermissionMode:       PermissionAcceptEdits,
		AllowedTools:         []string{"Bash(git *)"},
		DisallowedTools:      []string{"WebFetch"},
		Model:                "opus",
		FallbackModel:        "sonnet",
		MaxBudgetUSD:         10.00,
		SystemPrompt:         "You are helpful",
		AppendSystemPrompt:   "Always be concise",
		MaxTurns:             3,
		DisableSlashCommands: true,
		SettingSources:       []string{"user"},
		Debug:                "api",
	}

	args, err := buildArgs("complex prompt", opts, "/tmp/mcp.json")
	if err != nil {
		t.Fatalf("buildArgs() error: %v", err)
	}

	// Verify key flags
	assertContainsPair(t, args, "--permission-mode", "acceptEdits")
	assertContainsPair(t, args, "--model", "opus")
	assertContainsPair(t, args, "--fallback-model", "sonnet")
	assertContainsPair(t, args, "--max-budget-usd", "10.00")
	assertContainsPair(t, args, "--system-prompt", "You are helpful")
	assertContainsPair(t, args, "--append-system-prompt", "Always be concise")
	assertContainsPair(t, args, "--max-turns", "3")
	assertContains(t, args, "--disable-slash-commands")
	assertContainsPair(t, args, "--debug", "api")

	// Prompt is last
	if args[len(args)-1] != "complex prompt" {
		t.Errorf("prompt not last arg: got %q", args[len(args)-1])
	}
}

// ---------------------------------------------------------------------------
// Text extraction
// ---------------------------------------------------------------------------

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		msg  *StreamMessage
		want string
	}{
		{"nil message", nil, ""},
		{"direct text field", &StreamMessage{Text: "hello"}, "hello"},
		{"result type", &StreamMessage{Type: "result", Result: "final answer"}, "final answer"},
		{
			"message content",
			&StreamMessage{
				Message: &MessageContent{
					Content: []ContentBlock{{Type: "text", Text: "from content"}},
				},
			},
			"from content",
		},
		{
			"prefers direct text over content",
			&StreamMessage{
				Text: "direct",
				Message: &MessageContent{
					Content: []ContentBlock{{Type: "text", Text: "from content"}},
				},
			},
			"direct",
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

// ---------------------------------------------------------------------------
// Thinking extraction
// ---------------------------------------------------------------------------

func TestExtractThinking(t *testing.T) {
	tests := []struct {
		name string
		msg  *StreamMessage
		want string
	}{
		{"nil message", nil, ""},
		{"no thinking", &StreamMessage{Message: &MessageContent{
			Content: []ContentBlock{{Type: "text", Text: "hello"}},
		}}, ""},
		{"with thinking", &StreamMessage{Message: &MessageContent{
			Content: []ContentBlock{
				{Type: "thinking", Thinking: "let me consider this..."},
				{Type: "text", Text: "answer"},
			},
		}}, "let me consider this..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractThinking(tt.msg)
			if got != tt.want {
				t.Errorf("ExtractThinking() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractAllThinking(t *testing.T) {
	msg := &StreamMessage{Message: &MessageContent{
		Content: []ContentBlock{
			{Type: "thinking", Thinking: "first thought"},
			{Type: "text", Text: "response"},
			{Type: "thinking", Thinking: "second thought"},
		},
	}}

	got := ExtractAllThinking(msg)
	if got != "first thought\nsecond thought" {
		t.Errorf("ExtractAllThinking() = %q, want %q", got, "first thought\nsecond thought")
	}
}

// ---------------------------------------------------------------------------
// Todo extraction
// ---------------------------------------------------------------------------

func TestExtractTodos(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{
					Type: "tool_use",
					Name: "TodoWrite",
					Input: map[string]any{
						"todos": []any{
							map[string]any{"content": "Write tests", "status": "pending", "activeForm": "Writing tests"},
							map[string]any{"content": "Review code", "status": "completed"},
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

	if todos[0].Content != "Write tests" || todos[0].Status != "pending" || todos[0].ActiveForm != "Writing tests" {
		t.Errorf("todos[0] = %+v, unexpected values", todos[0])
	}
	if todos[1].Content != "Review code" || todos[1].Status != "completed" {
		t.Errorf("todos[1] = %+v, unexpected values", todos[1])
	}
}

// ---------------------------------------------------------------------------
// Tool extraction
// ---------------------------------------------------------------------------

func TestExtractBashCommand(t *testing.T) {
	msg := &StreamMessage{
		Message: &MessageContent{
			Content: []ContentBlock{
				{Type: "tool_use", Name: "Bash", Input: map[string]any{"command": "ls -la"}},
			},
		},
	}

	if cmd := ExtractBashCommand(msg); cmd != "ls -la" {
		t.Errorf("ExtractBashCommand() = %q, want %q", cmd, "ls -la")
	}
}

func TestExtractFileAccess(t *testing.T) {
	for _, toolName := range []string{"Read", "Write", "Edit"} {
		t.Run(toolName, func(t *testing.T) {
			msg := &StreamMessage{
				Message: &MessageContent{
					Content: []ContentBlock{
						{Type: "tool_use", Name: toolName, Input: map[string]any{"file_path": "/path/to/file.go"}},
					},
				},
			}

			if got := ExtractFileAccess(msg); got != "/path/to/file.go" {
				t.Errorf("ExtractFileAccess() = %q, want %q", got, "/path/to/file.go")
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
	if name := GetToolName(msg); name != "Bash" {
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
	if tools[0].Name != "Bash" || tools[1].Name != "Read" {
		t.Errorf("tools = %v, want Bash and Read", tools)
	}
}

// ---------------------------------------------------------------------------
// Structured output & usage extraction
// ---------------------------------------------------------------------------

func TestExtractStructuredOutput(t *testing.T) {
	t.Run("from result", func(t *testing.T) {
		output := map[string]any{"answer": "Paris", "confidence": 0.99}
		msg := &StreamMessage{Type: "result", StructuredOutput: output}
		got := ExtractStructuredOutput(msg)
		if got == nil {
			t.Fatal("ExtractStructuredOutput() returned nil")
		}
		m, ok := got.(map[string]any)
		if !ok {
			t.Fatal("ExtractStructuredOutput() not map[string]any")
		}
		if m["answer"] != "Paris" {
			t.Errorf("answer = %v, want Paris", m["answer"])
		}
	})

	t.Run("nil for non-result", func(t *testing.T) {
		msg := &StreamMessage{Type: "assistant"}
		if got := ExtractStructuredOutput(msg); got != nil {
			t.Errorf("ExtractStructuredOutput() = %v, want nil", got)
		}
	})
}

func TestExtractUsage(t *testing.T) {
	msg := &StreamMessage{
		Type: "result",
		Usage: &Usage{
			InputTokens:              1500,
			OutputTokens:             500,
			CacheCreationInputTokens: 100,
			CacheReadInputTokens:     200,
		},
	}

	u := ExtractUsage(msg)
	if u == nil {
		t.Fatal("ExtractUsage() returned nil")
	}
	if u.InputTokens != 1500 || u.OutputTokens != 500 {
		t.Errorf("usage = %+v, unexpected values", u)
	}
	if u.TotalTokens() != 2000 {
		t.Errorf("TotalTokens() = %d, want 2000", u.TotalTokens())
	}
}

func TestUsageTotalTokensNil(t *testing.T) {
	var u *Usage
	if u.TotalTokens() != 0 {
		t.Errorf("nil Usage.TotalTokens() = %d, want 0", u.TotalTokens())
	}
}

// ---------------------------------------------------------------------------
// Init message extraction
// ---------------------------------------------------------------------------

func TestExtractInitTools(t *testing.T) {
	t.Run("from init message", func(t *testing.T) {
		msg := &StreamMessage{
			Type:    "system",
			Subtype: "init",
			Tools:   []string{"Bash", "Read", "mcp__context7__resolve"},
		}

		tools := ExtractInitTools(msg)
		if len(tools) != 3 {
			t.Fatalf("ExtractInitTools() returned %d tools, want 3", len(tools))
		}
		if tools[0] != "Bash" || tools[2] != "mcp__context7__resolve" {
			t.Errorf("tools = %v, unexpected", tools)
		}
	})

	t.Run("nil for non-init", func(t *testing.T) {
		msg := &StreamMessage{Type: "assistant"}
		if got := ExtractInitTools(msg); got != nil {
			t.Errorf("ExtractInitTools() = %v, want nil", got)
		}
	})
}

func TestExtractInitPermissionMode(t *testing.T) {
	msg := &StreamMessage{
		Type:           "system",
		Subtype:        "init",
		PermissionMode: "bypassPermissions",
	}
	if got := ExtractInitPermissionMode(msg); got != "bypassPermissions" {
		t.Errorf("ExtractInitPermissionMode() = %q, want %q", got, "bypassPermissions")
	}
}

// ---------------------------------------------------------------------------
// Message type predicates
// ---------------------------------------------------------------------------

func TestIsMessageType(t *testing.T) {
	tests := []struct {
		msg         *StreamMessage
		isResult    bool
		isError     bool
		isAssistant bool
		isSystem    bool
		isInit      bool
		isUser      bool
	}{
		{&StreamMessage{Type: "result"}, true, false, false, false, false, false},
		{&StreamMessage{Type: "error"}, false, true, false, false, false, false},
		{&StreamMessage{Type: "assistant"}, false, false, true, false, false, false},
		{&StreamMessage{Type: "system"}, false, false, false, true, false, false},
		{&StreamMessage{Type: "system", Subtype: "init"}, false, false, false, true, true, false},
		{&StreamMessage{Type: "user"}, false, false, false, false, false, true},
		{nil, false, false, false, false, false, false},
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
		if got := IsSystem(tt.msg); got != tt.isSystem {
			t.Errorf("IsSystem(%v) = %v, want %v", tt.msg, got, tt.isSystem)
		}
		if got := IsInit(tt.msg); got != tt.isInit {
			t.Errorf("IsInit(%v) = %v, want %v", tt.msg, got, tt.isInit)
		}
		if got := IsUser(tt.msg); got != tt.isUser {
			t.Errorf("IsUser(%v) = %v, want %v", tt.msg, got, tt.isUser)
		}
	}
}

// ---------------------------------------------------------------------------
// ContentBlock methods
// ---------------------------------------------------------------------------

func TestContentBlockMethods(t *testing.T) {
	tests := []struct {
		block        ContentBlock
		isText       bool
		isToolUse    bool
		isThinking   bool
		isToolResult bool
	}{
		{ContentBlock{Type: "text", Text: "hello"}, true, false, false, false},
		{ContentBlock{Type: "tool_use", Name: "Bash"}, false, true, false, false},
		{ContentBlock{Type: "thinking", Thinking: "hmm..."}, false, false, true, false},
		{ContentBlock{Type: "tool_result", ToolUseID: "123"}, false, false, false, true},
	}

	for _, tt := range tests {
		if got := tt.block.IsText(); got != tt.isText {
			t.Errorf("%s.IsText() = %v, want %v", tt.block.Type, got, tt.isText)
		}
		if got := tt.block.IsToolUse(); got != tt.isToolUse {
			t.Errorf("%s.IsToolUse() = %v, want %v", tt.block.Type, got, tt.isToolUse)
		}
		if got := tt.block.IsThinking(); got != tt.isThinking {
			t.Errorf("%s.IsThinking() = %v, want %v", tt.block.Type, got, tt.isThinking)
		}
		if got := tt.block.IsToolResult(); got != tt.isToolResult {
			t.Errorf("%s.IsToolResult() = %v, want %v", tt.block.Type, got, tt.isToolResult)
		}
	}
}

// ---------------------------------------------------------------------------
// Message JSON parsing
// ---------------------------------------------------------------------------

func TestStreamMessageJSONParsing(t *testing.T) {
	t.Run("init message", func(t *testing.T) {
		raw := `{"type":"system","subtype":"init","session_id":"abc-123","model":"claude-sonnet-4-20250514","permissionMode":"bypassPermissions","tools":["Bash","Read","Grep"]}`

		var msg StreamMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if msg.Type != "system" || msg.Subtype != "init" {
			t.Errorf("type/subtype = %s/%s, want system/init", msg.Type, msg.Subtype)
		}
		if msg.SessionID != "abc-123" {
			t.Errorf("session_id = %q, want %q", msg.SessionID, "abc-123")
		}
		if msg.Model != "claude-sonnet-4-20250514" {
			t.Errorf("model = %q, want %q", msg.Model, "claude-sonnet-4-20250514")
		}
		if msg.PermissionMode != "bypassPermissions" {
			t.Errorf("permissionMode = %q, want %q", msg.PermissionMode, "bypassPermissions")
		}
		if len(msg.Tools) != 3 {
			t.Fatalf("tools length = %d, want 3", len(msg.Tools))
		}
		if msg.Tools[0] != "Bash" {
			t.Errorf("tools[0] = %q, want %q", msg.Tools[0], "Bash")
		}
	})

	t.Run("result message", func(t *testing.T) {
		raw := `{"type":"result","subtype":"success","session_id":"abc-123","result":"The answer is 4","cost_usd":0.01,"total_cost_usd":2.90,"duration_ms":12345,"duration_api_ms":10000,"is_error":false,"num_turns":5,"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":50,"cache_read_input_tokens":200}}`

		var msg StreamMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if msg.Type != "result" || msg.Subtype != "success" {
			t.Errorf("type/subtype = %s/%s, want result/success", msg.Type, msg.Subtype)
		}
		if msg.Result != "The answer is 4" {
			t.Errorf("result = %q", msg.Result)
		}
		if msg.CostUSD != 0.01 {
			t.Errorf("cost_usd = %f, want 0.01", msg.CostUSD)
		}
		if msg.TotalCost != 2.90 {
			t.Errorf("total_cost_usd = %f, want 2.90", msg.TotalCost)
		}
		if msg.DurationMS != 12345 {
			t.Errorf("duration_ms = %d, want 12345", msg.DurationMS)
		}
		if msg.DurationAPIMS != 10000 {
			t.Errorf("duration_api_ms = %d, want 10000", msg.DurationAPIMS)
		}
		if msg.NumTurns != 5 {
			t.Errorf("num_turns = %d, want 5", msg.NumTurns)
		}
		if msg.Usage == nil {
			t.Fatal("usage is nil")
		}
		if msg.Usage.InputTokens != 1000 || msg.Usage.OutputTokens != 500 {
			t.Errorf("usage = %+v", msg.Usage)
		}
		if msg.Usage.CacheCreationInputTokens != 50 || msg.Usage.CacheReadInputTokens != 200 {
			t.Errorf("cache tokens = %d/%d", msg.Usage.CacheCreationInputTokens, msg.Usage.CacheReadInputTokens)
		}
	})

	t.Run("assistant with thinking", func(t *testing.T) {
		raw := `{"type":"assistant","uuid":"msg-001","session_id":"abc-123","message":{"role":"assistant","content":[{"type":"thinking","thinking":"Let me think about this..."},{"type":"text","text":"The answer is 42."}]}}`

		var msg StreamMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if msg.UUID != "msg-001" {
			t.Errorf("uuid = %q, want %q", msg.UUID, "msg-001")
		}
		if msg.Message == nil || len(msg.Message.Content) != 2 {
			t.Fatal("unexpected message content")
		}
		if msg.Message.Content[0].Type != "thinking" || msg.Message.Content[0].Thinking != "Let me think about this..." {
			t.Errorf("thinking block = %+v", msg.Message.Content[0])
		}
		if msg.Message.Content[1].Type != "text" || msg.Message.Content[1].Text != "The answer is 42." {
			t.Errorf("text block = %+v", msg.Message.Content[1])
		}
	})

	t.Run("result with structured output", func(t *testing.T) {
		raw := `{"type":"result","subtype":"success","result":"","structured_output":{"answer":"Paris","confidence":0.99},"total_cost_usd":0.05}`

		var msg StreamMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if msg.StructuredOutput == nil {
			t.Fatal("structured_output is nil")
		}
		m, ok := msg.StructuredOutput.(map[string]any)
		if !ok {
			t.Fatal("structured_output is not map[string]any")
		}
		if m["answer"] != "Paris" {
			t.Errorf("answer = %v, want Paris", m["answer"])
		}
	})

	t.Run("error result", func(t *testing.T) {
		raw := `{"type":"result","subtype":"error_max_turns","is_error":true,"num_turns":3,"total_cost_usd":1.50}`

		var msg StreamMessage
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if !msg.IsErrorResult {
			t.Error("is_error should be true")
		}
		if msg.Subtype != "error_max_turns" {
			t.Errorf("subtype = %q, want error_max_turns", msg.Subtype)
		}
	})
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

func TestMetricsFromMessage(t *testing.T) {
	msg := &StreamMessage{
		Type:          "result",
		CostUSD:       0.05,
		TotalCost:     1.50,
		NumTurns:      3,
		DurationMS:    5000,
		DurationAPIMS: 4000,
		Model:         "claude-sonnet-4-20250514",
		SessionID:     "sess-123",
		Usage: &Usage{
			InputTokens:              2000,
			OutputTokens:             800,
			CacheCreationInputTokens: 100,
			CacheReadInputTokens:     300,
		},
	}

	m := metricsFromMessage(msg)

	if m.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f, want 0.05", m.CostUSD)
	}
	if m.TotalCostUSD != 1.50 {
		t.Errorf("TotalCostUSD = %f, want 1.50", m.TotalCostUSD)
	}
	if m.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", m.NumTurns)
	}
	if m.InputTokens != 2000 || m.OutputTokens != 800 {
		t.Errorf("tokens = %d/%d", m.InputTokens, m.OutputTokens)
	}
	if m.CacheCreationInputTokens != 100 || m.CacheReadInputTokens != 300 {
		t.Errorf("cache tokens = %d/%d", m.CacheCreationInputTokens, m.CacheReadInputTokens)
	}
	if m.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q", m.Model)
	}
	if m.SessionID != "sess-123" {
		t.Errorf("SessionID = %q", m.SessionID)
	}
}

func TestMetricsFromMessageNilUsage(t *testing.T) {
	msg := &StreamMessage{Type: "result", TotalCost: 0.50}
	m := metricsFromMessage(msg)
	if m.InputTokens != 0 || m.OutputTokens != 0 {
		t.Errorf("tokens should be 0 with nil usage")
	}
}

// ---------------------------------------------------------------------------
// Hooks nil safety
// ---------------------------------------------------------------------------

func TestHooksNilSafe(t *testing.T) {
	var h *Hooks

	// These should not panic
	h.invokeMessage(StreamMessage{})
	h.invokeText("text")
	h.invokeToolCall("tool", nil)
	h.invokeError(nil)
	h.invokeStart(0)
	h.invokeExit(0, 0)
	h.invokeMetrics(SessionMetrics{})

	// Empty hooks struct should also be safe
	h = &Hooks{}
	h.invokeMessage(StreamMessage{})
	h.invokeText("text")
	h.invokeToolCall("tool", nil)
	h.invokeError(nil)
	h.invokeStart(0)
	h.invokeExit(0, 0)
	h.invokeMetrics(SessionMetrics{})
}

func TestOnMetricsHook(t *testing.T) {
	var captured SessionMetrics
	h := &Hooks{
		OnMetrics: func(m SessionMetrics) {
			captured = m
		},
	}

	h.invokeMetrics(SessionMetrics{TotalCostUSD: 1.23, NumTurns: 5})

	if captured.TotalCostUSD != 1.23 || captured.NumTurns != 5 {
		t.Errorf("OnMetrics captured = %+v", captured)
	}
}

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

func TestErrors(t *testing.T) {
	pe := &ParseError{Line: "invalid json", Err: nil}
	if pe.Error() == "" {
		t.Error("ParseError.Error() returned empty string")
	}

	ee := &ExitError{Code: 1, Stderr: "something went wrong"}
	if ee.Error() == "" {
		t.Error("ExitError.Error() returned empty string")
	}

	se := &StartError{Err: ErrCLINotFound}
	if se.Error() == "" {
		t.Error("StartError.Error() returned empty string")
	}
	if se.Unwrap() != ErrCLINotFound {
		t.Error("StartError.Unwrap() didn't return wrapped error")
	}
}

// ---------------------------------------------------------------------------
// Type constants
// ---------------------------------------------------------------------------

func TestPermissionModeConstants(t *testing.T) {
	if string(PermissionDefault) != "default" {
		t.Errorf("PermissionDefault = %q", PermissionDefault)
	}
	if string(PermissionAcceptEdits) != "acceptEdits" {
		t.Errorf("PermissionAcceptEdits = %q", PermissionAcceptEdits)
	}
	if string(PermissionPlan) != "plan" {
		t.Errorf("PermissionPlan = %q", PermissionPlan)
	}
	if string(PermissionBypass) != "bypassPermissions" {
		t.Errorf("PermissionBypass = %q", PermissionBypass)
	}
}

func TestAgentDefinitionJSON(t *testing.T) {
	agent := AgentDefinition{
		Description: "Test agent",
		Prompt:      "You are a test agent",
		Tools:       []string{"Read", "Grep"},
		Model:       "sonnet",
	}

	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed AgentDefinition
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.Description != agent.Description || parsed.Prompt != agent.Prompt {
		t.Errorf("roundtrip mismatch: %+v", parsed)
	}
	if len(parsed.Tools) != 2 || parsed.Model != "sonnet" {
		t.Errorf("tools/model mismatch: %+v", parsed)
	}
}

func TestBoolPtr(t *testing.T) {
	truePtr := BoolPtr(true)
	falsePtr := BoolPtr(false)

	if truePtr == nil || !*truePtr {
		t.Error("BoolPtr(true) failed")
	}
	if falsePtr == nil || *falsePtr {
		t.Error("BoolPtr(false) failed")
	}
}

// ---------------------------------------------------------------------------
// Launcher and Session init
// ---------------------------------------------------------------------------

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
	if s.Messages == nil || s.Text == nil || s.Errors == nil {
		t.Error("channels should not be nil")
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

func TestSessionConfigEmbedding(t *testing.T) {
	// Verify that embedded LaunchOptions fields are accessible
	cfg := SessionConfig{}
	cfg.Model = "sonnet"
	cfg.SkipPermissions = true
	cfg.MaxTurns = 5
	cfg.ID = "my-session"
	cfg.ChannelBuffer = 200

	if cfg.Model != "sonnet" {
		t.Errorf("cfg.Model = %q, want sonnet", cfg.Model)
	}
	if !cfg.SkipPermissions {
		t.Error("cfg.SkipPermissions should be true")
	}
	if cfg.MaxTurns != 5 {
		t.Errorf("cfg.MaxTurns = %d, want 5", cfg.MaxTurns)
	}

	// Verify LaunchOptions can be accessed directly
	opts := cfg.LaunchOptions
	if opts.Model != "sonnet" || !opts.SkipPermissions || opts.MaxTurns != 5 {
		t.Errorf("LaunchOptions = %+v, unexpected", opts)
	}
}

func TestSessionCurrentMetrics(t *testing.T) {
	s, _ := NewSession(SessionConfig{})
	m := s.CurrentMetrics()
	if m.TotalCostUSD != 0 || m.NumTurns != 0 || m.InputTokens != 0 {
		t.Errorf("initial metrics should be zero: %+v", m)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require Claude CLI)
// ---------------------------------------------------------------------------

func skipIfNoCLI(t *testing.T) {
	t.Helper()
	if !CLIAvailable() {
		t.Skip("Claude CLI not available - skipping integration test")
	}
}

func TestIntegrationSimplePrompt(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			MaxTurns:        1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// A prompt designed to get a predictable, short answer
	text, err := session.CollectAll(ctx, "What is 2+2? Reply with ONLY the number, nothing else.")
	if err != nil {
		t.Fatalf("CollectAll() error: %v", err)
	}

	t.Logf("Response: %q", text)

	// The response should contain "4"
	if !strings.Contains(text, "4") {
		t.Errorf("expected response to contain '4', got: %q", text)
	}
}

func TestIntegrationRunAndCollect(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			MaxTurns:        1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := session.RunAndCollect(ctx, "What is the capital of Japan? Reply in one word.")
	if err != nil {
		t.Fatalf("RunAndCollect() error: %v", err)
	}

	t.Logf("Text: %q", result.Text)
	t.Logf("TotalCost: $%.6f", result.TotalCost)
	t.Logf("Model: %s", result.Model)
	t.Logf("SessionID: %s", result.SessionID)
	t.Logf("NumTurns: %d", result.NumTurns)
	t.Logf("Duration: %s", result.Duration)
	if result.Usage != nil {
		t.Logf("Usage: in=%d out=%d", result.Usage.InputTokens, result.Usage.OutputTokens)
	}

	// Validate response content
	if !strings.Contains(strings.ToLower(result.Text), "tokyo") {
		t.Errorf("expected response to contain 'tokyo', got: %q", result.Text)
	}

	// Validate metrics
	if result.TotalCost <= 0 {
		t.Error("TotalCost should be positive")
	}
	if result.Model == "" {
		t.Error("Model should not be empty")
	}
	if result.SessionID == "" {
		t.Error("SessionID should not be empty")
	}
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}

	// Validate metrics snapshot
	metrics := session.CurrentMetrics()
	t.Logf("CurrentMetrics: cost=$%.6f tokens=%d+%d", metrics.TotalCostUSD, metrics.InputTokens, metrics.OutputTokens)
	if metrics.TotalCostUSD <= 0 {
		t.Error("CurrentMetrics().TotalCostUSD should be positive")
	}
}

func TestIntegrationSystemPrompt(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			SystemPrompt:    "You must respond to every message with exactly the word PONG and nothing else. No punctuation, no explanation.",
			MaxTurns:        1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	text, err := session.CollectAll(ctx, "PING")
	if err != nil {
		t.Fatalf("CollectAll() error: %v", err)
	}

	t.Logf("Response: %q", text)

	// With the system prompt, Claude should respond with "PONG"
	if !strings.Contains(strings.ToUpper(text), "PONG") {
		t.Errorf("expected response to contain 'PONG', got: %q", text)
	}
}

func TestIntegrationAppendSystemPrompt(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions:    true,
			AppendSystemPrompt: "IMPORTANT: You must end every response with the exact word TERMINUS on its own line.",
			MaxTurns:           1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	text, err := session.CollectAll(ctx, "Say hello briefly.")
	if err != nil {
		t.Fatalf("CollectAll() error: %v", err)
	}

	t.Logf("Response: %q", text)

	if !strings.Contains(text, "TERMINUS") {
		t.Errorf("expected response to end with 'TERMINUS', got: %q", text)
	}
}

func TestIntegrationMaxTurns(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			MaxTurns:        1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := session.RunAndCollect(ctx, "Say hello.")
	if err != nil {
		t.Fatalf("RunAndCollect() error: %v", err)
	}

	t.Logf("NumTurns: %d", result.NumTurns)
	t.Logf("Text: %q", result.Text)

	// Should complete (not hang)
	if result.Text == "" {
		t.Error("expected non-empty response")
	}
}

func TestIntegrationStreamingWithHooks(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	var gotStart, gotExit, gotMessage, gotText, gotMetrics bool

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			MaxTurns:        1,
			Hooks: &Hooks{
				OnStart:   func(pid int) { gotStart = true },
				OnExit:    func(code int, duration time.Duration) { gotExit = true },
				OnMessage: func(msg StreamMessage) { gotMessage = true },
				OnText:    func(text string) { gotText = true },
				OnMetrics: func(m SessionMetrics) { gotMetrics = true },
			},
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	_, err = session.CollectAll(ctx, "Say exactly: hello world")
	if err != nil {
		t.Fatalf("CollectAll() error: %v", err)
	}

	// Wait a moment for hooks to fire
	<-session.Done()

	if !gotStart {
		t.Error("OnStart hook not called")
	}
	if !gotMessage {
		t.Error("OnMessage hook not called")
	}
	if !gotText {
		t.Error("OnText hook not called")
	}
	if !gotMetrics {
		t.Error("OnMetrics hook not called")
	}
	if !gotExit {
		t.Log("OnExit hook may not have fired yet (race with check)")
	}
}

func TestIntegrationCollectMessages(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			SkipPermissions: true,
			MaxTurns:        1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	msgs, err := session.CollectMessages(ctx, "What is 1+1? Reply with just the number.")
	if err != nil {
		t.Fatalf("CollectMessages() error: %v", err)
	}

	t.Logf("Received %d messages", len(msgs))
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}

	// Should have at least an init message and a result message
	var hasInit, hasResult bool
	for _, msg := range msgs {
		t.Logf("  type=%s subtype=%s", msg.Type, msg.Subtype)
		if IsInit(&msg) {
			hasInit = true
			t.Logf("  init: model=%s permissionMode=%s tools=%d",
				msg.Model, msg.PermissionMode, len(msg.Tools))
		}
		if IsResult(&msg) {
			hasResult = true
			t.Logf("  result: cost=$%.6f turns=%d", msg.TotalCost, msg.NumTurns)
			if msg.Usage != nil {
				t.Logf("  usage: in=%d out=%d", msg.Usage.InputTokens, msg.Usage.OutputTokens)
			}
		}
	}

	if !hasInit {
		t.Error("no init message found")
	}
	if !hasResult {
		t.Error("no result message found")
	}
}

func TestIntegrationPermissionMode(t *testing.T) {
	skipIfNoCLI(t)
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	session, err := NewSession(SessionConfig{
		LaunchOptions: LaunchOptions{
			PermissionMode: PermissionBypass,
			MaxTurns:       1,
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	msgs, err := session.CollectMessages(ctx, "Say hello.")
	if err != nil {
		t.Fatalf("CollectMessages() error: %v", err)
	}

	// Check that the init message reports the correct permission mode
	for _, msg := range msgs {
		if IsInit(&msg) {
			t.Logf("Init permissionMode: %q", msg.PermissionMode)
			if msg.PermissionMode != "bypassPermissions" {
				t.Errorf("expected bypassPermissions, got %q", msg.PermissionMode)
			}
			return
		}
	}
	t.Error("no init message found to verify permission mode")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func assertContains(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			return
		}
	}
	t.Errorf("args should contain %q, got: %v", flag, args)
}

func assertNotContains(t *testing.T, args []string, flag string) {
	t.Helper()
	for _, a := range args {
		if a == flag {
			t.Errorf("args should NOT contain %q, got: %v", flag, args)
			return
		}
	}
}

func assertContainsPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return
		}
	}
	t.Errorf("args should contain %q %q, got: %v", flag, value, args)
}

func indexOfArg(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}
