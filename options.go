package claude

import "time"

// LaunchOptions configures a Claude CLI launch.
type LaunchOptions struct {
	// APIKey sets ANTHROPIC_API_KEY for this session.
	// When set, billing uses pay-as-you-go API rates instead of subscription.
	// Leave empty to use the machine's existing CLI authentication
	// (browser OAuth or setup-token).
	APIKey string

	// SkipPermissions bypasses permission prompts (--dangerously-skip-permissions).
	// Use only in trusted/automated environments.
	SkipPermissions bool

	// WorkDir sets the working directory for Claude.
	// Defaults to current directory if empty.
	WorkDir string

	// Model specifies which Claude model to use (e.g., "opus", "sonnet", "haiku").
	// Defaults to Claude's default model if empty.
	Model string

	// SystemPrompt provides a system message to Claude.
	SystemPrompt string

	// MaxTurns limits the number of agentic turns.
	// Zero means no limit.
	MaxTurns int

	// Timeout sets the maximum duration for the session.
	// Zero means no timeout.
	Timeout time.Duration

	// Verbose enables verbose output from Claude CLI.
	Verbose bool

	// AdditionalArgs allows passing extra CLI arguments.
	// Use sparingly; prefer structured options.
	AdditionalArgs []string

	// Hooks for observability. Nil is safe.
	Hooks *Hooks
}

// SessionConfig configures a high-level Session.
type SessionConfig struct {
	// APIKey sets ANTHROPIC_API_KEY for this session.
	// When set, billing uses pay-as-you-go API rates instead of subscription.
	// Leave empty to use the machine's existing CLI authentication.
	APIKey string

	// ID is an optional identifier for this session.
	// Useful for logging and debugging.
	ID string

	// WorkDir sets the working directory for Claude.
	WorkDir string

	// Model specifies which Claude model to use.
	Model string

	// SystemPrompt provides a system message to Claude.
	SystemPrompt string

	// SkipPermissions bypasses permission prompts.
	SkipPermissions bool

	// MaxTurns limits the number of agentic turns.
	MaxTurns int

	// Timeout sets the maximum duration for the session.
	Timeout time.Duration

	// Verbose enables verbose output from Claude CLI.
	Verbose bool

	// ChannelBuffer sets the buffer size for message channels.
	// Defaults to 100 if zero.
	ChannelBuffer int

	// Hooks for observability. Nil is safe.
	Hooks *Hooks
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
