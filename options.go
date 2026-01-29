package claude

import "time"

// PermissionMode controls how Claude handles tool permission requests.
type PermissionMode string

const (
	// PermissionDefault prompts for permission on first use of each tool.
	PermissionDefault PermissionMode = "default"

	// PermissionAcceptEdits auto-approves file operations (Read/Edit/Write)
	// without prompting; other tools still require permission.
	PermissionAcceptEdits PermissionMode = "acceptEdits"

	// PermissionPlan enables planning mode — Claude can analyze files
	// but cannot modify files or execute commands.
	PermissionPlan PermissionMode = "plan"

	// PermissionBypass auto-approves ALL tool uses without prompts.
	// Hooks still execute. Use with extreme caution.
	// Equivalent to --dangerously-skip-permissions.
	PermissionBypass PermissionMode = "bypassPermissions"
)

// AgentDefinition defines a custom subagent that Claude can invoke via the Task tool.
//
// Example:
//
//	claude.AgentDefinition{
//		Description: "Expert code reviewer",
//		Prompt:      "You are a senior code reviewer. Focus on correctness and security.",
//		Tools:       []string{"Read", "Grep", "Glob"},
//		Model:       "sonnet",
//	}
type AgentDefinition struct {
	// Description summarizes what this agent does (required).
	Description string `json:"description"`

	// Prompt is the system instruction for this agent (required).
	Prompt string `json:"prompt"`

	// Tools restricts which tools this agent can use.
	// If nil, the agent inherits all available tools.
	Tools []string `json:"tools,omitempty"`

	// Model overrides the model for this agent.
	// Values: "sonnet", "opus", "haiku", "inherit", or a full model name.
	Model string `json:"model,omitempty"`
}

// SandboxSettings configures command sandboxing for security.
// Pass via the Settings field as JSON since sandbox is configured through settings.
//
// Example:
//
//	import "encoding/json"
//	sb, _ := json.Marshal(map[string]any{
//		"sandbox": claude.SandboxSettings{Enabled: true, AutoAllowBashIfSandboxed: true},
//	})
//	opts := claude.LaunchOptions{Settings: string(sb)}
type SandboxSettings struct {
	// Enabled activates sandboxing.
	Enabled bool `json:"enabled"`

	// AutoAllowBashIfSandboxed auto-approves Bash tool when sandbox is active.
	AutoAllowBashIfSandboxed bool `json:"autoAllowBashIfSandboxed,omitempty"`
}

// LaunchOptions configures a Claude CLI launch.
//
// Fields map directly to CLI flags. Use AdditionalArgs as an escape hatch
// for flags not yet exposed as struct fields.
type LaunchOptions struct {
	// --- Authentication ---

	// APIKey sets ANTHROPIC_API_KEY for this session.
	// When set, billing uses pay-as-you-go API rates instead of subscription.
	// Leave empty to use the machine's existing CLI authentication.
	APIKey string

	// --- Permission & Security ---

	// PermissionMode controls how Claude handles tool permissions.
	// Takes precedence over SkipPermissions if both are set.
	// See PermissionDefault, PermissionAcceptEdits, PermissionPlan, PermissionBypass.
	PermissionMode PermissionMode

	// SkipPermissions bypasses all permission prompts (--dangerously-skip-permissions).
	// Shortcut for PermissionMode = PermissionBypass.
	// PermissionMode takes precedence if both are set.
	SkipPermissions bool

	// AllowDangerouslySkipPermissions enables permission bypassing as an option
	// without activating it. Used with PermissionPlan to allow escalation.
	AllowDangerouslySkipPermissions bool

	// AllowedTools lists tools that execute without permission prompts.
	// Supports specifiers: "Bash(git log *)", "Read(~/.zshrc)", "Edit(./src/**)".
	AllowedTools []string

	// DisallowedTools lists tools removed from context entirely.
	// These tools cannot be used regardless of permission mode.
	DisallowedTools []string

	// PermissionPromptTool specifies an MCP tool to handle permission
	// prompts in non-interactive (print) mode.
	PermissionPromptTool string

	// --- Model & Budget ---

	// Model specifies which Claude model to use.
	// Values: "opus", "sonnet", "haiku", or a full model name like
	// "claude-sonnet-4-20250514".
	Model string

	// FallbackModel enables automatic fallback when the default model is overloaded.
	FallbackModel string

	// MaxBudgetUSD sets the maximum dollar amount to spend on API calls.
	// Zero means no budget limit.
	MaxBudgetUSD float64

	// MaxThinkingTokens overrides the thinking token budget.
	// Set via MAX_THINKING_TOKENS environment variable.
	// Default is 31999, max is 128000.
	MaxThinkingTokens int

	// Betas lists beta features to enable.
	// Example: []string{"interleaved-thinking", "context-1m-2025-08-07"}
	Betas []string

	// --- System Prompt ---

	// SystemPrompt replaces the entire default system prompt with custom text.
	// Mutually exclusive with SystemPromptFile.
	SystemPrompt string

	// SystemPromptFile loads the system prompt from a file path.
	// Mutually exclusive with SystemPrompt. Print mode only.
	SystemPromptFile string

	// AppendSystemPrompt appends text to the end of the default system prompt.
	// Can be combined with SystemPrompt or SystemPromptFile.
	// This is the safest way to add custom instructions while keeping defaults.
	AppendSystemPrompt string

	// AppendSystemPromptFile loads additional system prompt text from a file.
	// Print mode only.
	AppendSystemPromptFile string

	// --- Session Management ---

	// Resume resumes a session by ID or name.
	Resume string

	// Continue continues the most recent conversation in the working directory.
	Continue bool

	// ForkSession creates a new session ID when resuming instead of reusing
	// the original. Use with Resume.
	ForkSession bool

	// SessionID uses a specific session ID (must be valid UUID).
	// Useful for deterministic session management.
	SessionID string

	// NoSessionPersistence disables session persistence.
	// Sessions are not saved to disk and cannot be resumed.
	NoSessionPersistence bool

	// --- Tools & Agents ---

	// Tools restricts which built-in tools Claude can use.
	// nil = use defaults (all tools). Specific tools: []string{"Bash", "Edit", "Read"}.
	// For disabling all tools, use AdditionalArgs: []string{"--tools", ""}.
	Tools []string

	// Agents defines custom subagents that Claude can invoke via the Task tool.
	// The map key is the agent name used in Task tool calls.
	//
	// Example:
	//
	//	Agents: map[string]claude.AgentDefinition{
	//		"reviewer": {
	//			Description: "Reviews code for bugs",
	//			Prompt:      "You are a code reviewer...",
	//			Tools:       []string{"Read", "Grep"},
	//			Model:       "sonnet",
	//		},
	//	}
	Agents map[string]AgentDefinition

	// DisableSlashCommands disables all skills and slash commands for the session.
	DisableSlashCommands bool

	// --- Input/Output ---

	// JSONSchema requests validated JSON output matching the given schema.
	// The value should be a JSON Schema (map[string]any or a struct).
	// When set, the result message's StructuredOutput field contains validated data.
	//
	// Example:
	//
	//	JSONSchema: map[string]any{
	//		"type": "object",
	//		"properties": map[string]any{
	//			"answer": map[string]any{"type": "string"},
	//			"confidence": map[string]any{"type": "number"},
	//		},
	//		"required": []string{"answer"},
	//	}
	JSONSchema any

	// IncludePartialMessages includes partial streaming events in output.
	// Requires --print and --output-format=stream-json (both always set by SDK).
	IncludePartialMessages bool

	// InputFormat specifies the input format: "text" (default) or "stream-json".
	InputFormat string

	// --- Configuration ---

	// SettingSources specifies which settings to load.
	// Values: "user", "project", "local". Example: []string{"user", "project"}.
	SettingSources []string

	// Settings is a path to a settings JSON file or an inline JSON string.
	// Overrides settings from SettingSources.
	Settings string

	// PluginDirs specifies directories to load plugins from.
	PluginDirs []string

	// AddDirs adds additional working directories for Claude to access.
	AddDirs []string

	// --- Environment ---

	// WorkDir sets the working directory for Claude.
	// Defaults to the current directory if empty.
	WorkDir string

	// Env sets additional environment variables for the CLI process.
	// These are merged with the current process environment.
	// Keys that already exist are overwritten.
	Env map[string]string

	// --- Limits ---

	// MaxTurns limits the number of agentic turns.
	// Zero means no limit.
	MaxTurns int

	// Timeout sets the maximum duration for the session.
	// Zero means no timeout.
	Timeout time.Duration

	// --- MCP ---

	// MCPServers configures MCP servers for this session.
	// Each entry maps a server name to its configuration.
	// Passed to Claude via --mcp-config as a temp JSON file.
	//
	// Example:
	//
	//	MCPServers: map[string]claude.MCPServer{
	//		"context7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
	//	}
	MCPServers map[string]MCPServer

	// StrictMCP when true, only uses MCP servers from MCPServers,
	// ignoring all other configured MCP servers.
	StrictMCP bool

	// --- Debug ---

	// Verbose enables verbose logging.
	// Note: always enabled internally for stream-json output.
	Verbose bool

	// Debug enables debug mode with optional category filtering.
	// Example: "api,mcp" to debug API and MCP categories.
	Debug string

	// Chrome controls Chrome browser integration.
	// nil = use default, BoolPtr(true) = enable, BoolPtr(false) = disable.
	Chrome *bool

	// --- Advanced ---

	// AdditionalArgs allows passing extra CLI arguments not yet exposed
	// as struct fields. Use sparingly; prefer structured options.
	AdditionalArgs []string

	// Hooks provides optional callbacks for observability.
	// Nil is safe — all hooks are nil-checked before invocation.
	Hooks *Hooks
}

// SessionConfig configures a high-level Session.
//
// It embeds LaunchOptions for all CLI configuration, plus adds
// session-specific fields like ID and ChannelBuffer.
//
// Example:
//
//	cfg := claude.SessionConfig{
//		LaunchOptions: claude.LaunchOptions{
//			Model:           "sonnet",
//			SkipPermissions: true,
//			MaxTurns:        10,
//		},
//		ID:            "my-session",
//		ChannelBuffer: 200,
//	}
type SessionConfig struct {
	LaunchOptions

	// ID is an optional identifier for this session.
	// Used for logging and debugging. Not the CLI session UUID — use
	// LaunchOptions.SessionID for that.
	// Auto-generated if empty.
	ID string

	// ChannelBuffer sets the buffer size for message channels.
	// Defaults to 100 if zero.
	ChannelBuffer int
}

// MCPServer configures an MCP server for a Claude session.
//
// Supports three transport types:
//   - stdio (default): Set Command and Args to spawn a subprocess
//   - HTTP: Set Type to "http" and provide URL
//   - SSE (deprecated): Set Type to "sse" and provide URL
//
// Example (stdio):
//
//	claude.MCPServer{
//		Command: "npx",
//		Args:    []string{"-y", "@upstash/context7-mcp"},
//	}
//
// Example (HTTP):
//
//	claude.MCPServer{
//		Type: "http",
//		URL:  "https://api.example.com/mcp/",
//		Headers: map[string]string{
//			"Authorization": "Bearer token",
//		},
//	}
type MCPServer struct {
	// Command is the executable to run (stdio transport).
	Command string `json:"command,omitempty"`

	// Args are the command arguments (stdio transport).
	Args []string `json:"args,omitempty"`

	// Env sets environment variables for the server process (stdio transport).
	Env map[string]string `json:"env,omitempty"`

	// Type is the transport type: "http" or "sse".
	// Leave empty for stdio (default).
	Type string `json:"type,omitempty"`

	// URL is the server endpoint (http/sse transport).
	URL string `json:"url,omitempty"`

	// Headers are HTTP headers (http/sse transport).
	Headers map[string]string `json:"headers,omitempty"`
}

// Hooks provides optional callbacks for observability.
//
// All hooks are called synchronously in the message processing goroutine.
// Keep hook implementations fast to avoid blocking message processing.
// Nil hooks are safely ignored.
type Hooks struct {
	// OnMessage is called for every parsed StreamMessage.
	OnMessage func(StreamMessage)

	// OnText is called when text content is extracted from a message.
	OnText func(text string)

	// OnToolCall is called when Claude invokes a tool.
	// The name is the tool name, input is the tool arguments.
	OnToolCall func(name string, input map[string]any)

	// OnError is called for non-fatal errors (e.g., parse errors).
	// Fatal errors are returned from methods directly.
	OnError func(error)

	// OnStart is called when the CLI process starts.
	OnStart func(pid int)

	// OnExit is called when the CLI process exits.
	// code is the exit code, duration is the total runtime.
	OnExit func(code int, duration time.Duration)

	// OnMetrics is called when session metrics are available (from result messages).
	OnMetrics func(SessionMetrics)
}

// invoke safely calls a hook if it's not nil.
func (h *Hooks) invokeMessage(msg StreamMessage) {
	if h != nil && h.OnMessage != nil {
		h.OnMessage(msg)
	}
}

func (h *Hooks) invokeText(text string) {
	if h != nil && h.OnText != nil {
		h.OnText(text)
	}
}

func (h *Hooks) invokeToolCall(name string, input map[string]any) {
	if h != nil && h.OnToolCall != nil {
		h.OnToolCall(name, input)
	}
}

func (h *Hooks) invokeError(err error) {
	if h != nil && h.OnError != nil {
		h.OnError(err)
	}
}

func (h *Hooks) invokeStart(pid int) {
	if h != nil && h.OnStart != nil {
		h.OnStart(pid)
	}
}

func (h *Hooks) invokeExit(code int, duration time.Duration) {
	if h != nil && h.OnExit != nil {
		h.OnExit(code, duration)
	}
}

func (h *Hooks) invokeMetrics(m SessionMetrics) {
	if h != nil && h.OnMetrics != nil {
		h.OnMetrics(m)
	}
}

// SessionMetrics contains accumulated session metrics.
//
// Available via Session.CurrentMetrics() (sync) or the OnMetrics hook (async).
// Populated from result messages as they arrive.
type SessionMetrics struct {
	// CostUSD is the cost for this specific invocation.
	CostUSD float64

	// TotalCostUSD is the total accumulated cost across the session.
	TotalCostUSD float64

	// NumTurns is the number of agentic turns completed.
	NumTurns int

	// InputTokens is the total input tokens consumed.
	InputTokens int

	// OutputTokens is the total output tokens generated.
	OutputTokens int

	// CacheCreationInputTokens is the number of tokens used to create cache entries.
	CacheCreationInputTokens int

	// CacheReadInputTokens is the number of tokens read from cache.
	CacheReadInputTokens int

	// DurationMS is the total duration in milliseconds.
	DurationMS int64

	// DurationAPIMS is the API-only duration in milliseconds.
	DurationAPIMS int64

	// Model is the model used for the session.
	Model string

	// SessionID is the CLI session identifier.
	SessionID string
}

// BoolPtr returns a pointer to a bool value.
// Useful for the Chrome field which uses tri-state logic (nil/true/false).
func BoolPtr(v bool) *bool {
	return &v
}
