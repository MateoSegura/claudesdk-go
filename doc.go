// Package claude provides a programmatic Go interface to the Claude CLI.
//
// This SDK wraps the Claude Code CLI (@anthropic-ai/claude-code) as a subprocess,
// parsing its stream-json output into typed Go structs. Zero external dependencies.
//
// # Two-Tier API
//
// The SDK offers two levels of abstraction:
//
// Session (high-level, recommended) manages goroutines internally and provides
// Go channels for async message consumption. Four collection methods cover
// different use cases:
//
//	session, err := claude.NewSession(claude.SessionConfig{
//		LaunchOptions: claude.LaunchOptions{
//			SkipPermissions: true,
//			Model:           "sonnet",
//			MaxTurns:        10,
//		},
//	})
//
//	// Simple text response
//	text, err := session.CollectAll(ctx, "What is 2+2?")
//
//	// All messages with metadata
//	msgs, err := session.CollectMessages(ctx, "Analyze this code")
//
//	// Full result with cost, tokens, and metrics
//	result, err := session.RunAndCollect(ctx, "Explain recursion")
//
//	// Non-blocking streaming via channels
//	session.Run(ctx, "Write a haiku")
//	for msg := range session.Messages { ... }
//
// Launcher (low-level) provides synchronous message reading and direct
// process control for custom read loops:
//
//	launcher := claude.NewLauncher()
//	err := launcher.Start(ctx, "Explain Go interfaces", claude.LaunchOptions{
//		SkipPermissions: true,
//	})
//	defer launcher.Wait()
//
//	for {
//		msg, err := launcher.ReadMessage()
//		if msg == nil { break }
//		fmt.Print(claude.ExtractText(msg))
//	}
//
// # Configuration
//
// [LaunchOptions] provides 30+ fields mapping directly to CLI flags, organized
// into categories: authentication, permissions, model/budget, system prompt,
// session management, tools/agents, input/output, MCP servers, and debug.
//
// [SessionConfig] embeds LaunchOptions and adds session-specific fields (ID,
// channel buffer size).
//
// # Permission Modes
//
// Four permission modes control tool approval behavior:
//
//   - [PermissionDefault]: Prompt for each tool on first use
//   - [PermissionAcceptEdits]: Auto-approve file operations (Read/Edit/Write)
//   - [PermissionPlan]: Read-only mode, no mutations or commands
//   - [PermissionBypass]: Auto-approve all tools (use with caution)
//
// Granular control is available via AllowedTools and DisallowedTools with
// glob-pattern support (e.g., "Bash(git log *)").
//
// # Real-Time Metrics
//
// Session metrics (cost, tokens, turns, model, duration) are available via:
//
// Synchronous polling (thread-safe):
//
//	metrics := session.CurrentMetrics()
//	fmt.Printf("Cost: $%.4f, Tokens: %d+%d\n",
//		metrics.TotalCostUSD, metrics.InputTokens, metrics.OutputTokens)
//
// Asynchronous push via hooks:
//
//	Hooks: &claude.Hooks{
//		OnMetrics: func(m claude.SessionMetrics) {
//			log.Printf("$%.4f | %d turns", m.TotalCostUSD, m.NumTurns)
//		},
//	}
//
// # Message Extraction
//
// Typed helper functions extract structured data from stream messages:
//
//   - [ExtractText], [ExtractAllText]: Text content from any message type
//   - [ExtractThinking], [ExtractAllThinking]: Reasoning/thinking blocks
//   - [ExtractTodos]: TodoWrite tool call items
//   - [ExtractBashCommand]: Commands from Bash tool calls
//   - [ExtractFileAccess], [ExtractAllFileAccess]: File paths from Read/Write/Edit
//   - [ExtractStructuredOutput]: Validated JSON from --json-schema
//   - [ExtractUsage]: Token consumption data
//   - [ExtractInitTools]: Available tools from init message
//   - [GetToolName], [GetToolCall], [GetAllToolCalls]: Tool invocation details
//
// Type predicates ([IsResult], [IsAssistant], [IsInit], etc.) simplify
// message filtering in stream processing loops.
//
// # Hooks
//
// Optional [Hooks] callbacks provide observability without coupling to a logging
// framework. Seven hooks cover the complete lifecycle:
//
//	&claude.Hooks{
//		OnStart:    func(pid int) { ... },            // Process started
//		OnMessage:  func(msg StreamMessage) { ... },   // Any message parsed
//		OnText:     func(text string) { ... },         // Text extracted
//		OnToolCall: func(name string, input map[string]any) { ... }, // Tool invoked
//		OnMetrics:  func(m SessionMetrics) { ... },    // Metrics available
//		OnError:    func(err error) { ... },           // Non-fatal error
//		OnExit:     func(code int, d time.Duration) { ... }, // Process exited
//	}
//
// All hooks and the Hooks pointer itself are nil-safe.
//
// # MCP Servers
//
// External tool providers are configured via [MCPServer] and passed to Claude
// through the MCPServers field. Supports stdio, HTTP, and SSE transports:
//
//	MCPServers: map[string]claude.MCPServer{
//		"context7": {Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}},
//		"my-api":   {Type: "http", URL: "https://api.example.com/mcp/"},
//	}
//
// # Custom Agents
//
// Define specialized subagents via [AgentDefinition] that Claude can invoke
// through the Task tool:
//
//	Agents: map[string]claude.AgentDefinition{
//		"reviewer": {
//			Description: "Reviews code for bugs",
//			Prompt:      "You are a code reviewer...",
//			Tools:       []string{"Read", "Grep"},
//			Model:       "sonnet",
//		},
//	}
//
// # Requirements
//
// The Claude CLI must be installed and available in PATH:
//
//	npm install -g @anthropic-ai/claude-code
//
// Use [CLIAvailable] to check, or [MustCLIAvailable] for fail-fast:
//
//	func init() {
//		claude.MustCLIAvailable()
//	}
package claude
