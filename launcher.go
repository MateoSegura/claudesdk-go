package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Launcher provides low-level control over a Claude CLI subprocess.
//
// Launcher is the foundation of the SDK, offering synchronous message reading
// and direct process control. Use this when you need fine-grained control
// over the read loop or process lifecycle.
//
// For a higher-level channel-based API, see Session.
//
// Example:
//
//	launcher := claude.NewLauncher()
//	err := launcher.Start(ctx, "Explain recursion", claude.LaunchOptions{})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer launcher.Wait()
//
//	for {
//		msg, err := launcher.ReadMessage()
//		if err != nil {
//			log.Fatal(err)
//		}
//		if msg == nil {
//			break // EOF
//		}
//		fmt.Println(claude.ExtractText(msg))
//	}
type Launcher struct {
	cmd       *exec.Cmd
	stdout    *bufio.Scanner
	stderr    io.ReadCloser
	stderrBuf []byte
	startTime time.Time
	hooks     *Hooks
	tempFiles []string // temp files cleaned up on Wait

	mu      sync.Mutex
	started bool
	done    chan struct{}
}

// NewLauncher creates a new Launcher.
//
// The launcher is not started until Start is called.
func NewLauncher() *Launcher {
	return &Launcher{
		done: make(chan struct{}),
	}
}

// buildArgs constructs CLI arguments from LaunchOptions.
// mcpConfigFile is the path to a temporary MCP config file (empty if none).
// The prompt is appended at the end.
func buildArgs(prompt string, opts LaunchOptions, mcpConfigFile string) ([]string, error) {
	// Required flags for SDK mode
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
	}

	// --- Permission & Security ---

	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", string(opts.PermissionMode))
	} else if opts.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}

	if opts.AllowDangerouslySkipPermissions {
		args = append(args, "--allow-dangerously-skip-permissions")
	}

	for _, t := range opts.AllowedTools {
		args = append(args, "--allowedTools", t)
	}

	for _, t := range opts.DisallowedTools {
		args = append(args, "--disallowedTools", t)
	}

	if opts.PermissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", opts.PermissionPromptTool)
	}

	// --- Model & Budget ---

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(opts.MaxBudgetUSD, 'f', 2, 64))
	}

	if len(opts.Betas) > 0 {
		args = append(args, "--betas", strings.Join(opts.Betas, ","))
	}

	// --- System Prompt ---

	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	} else if opts.SystemPromptFile != "" {
		args = append(args, "--system-prompt-file", opts.SystemPromptFile)
	}

	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}

	if opts.AppendSystemPromptFile != "" {
		args = append(args, "--append-system-prompt-file", opts.AppendSystemPromptFile)
	}

	// --- Session Management ---

	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	if opts.Continue {
		args = append(args, "--continue")
	}

	if opts.ForkSession {
		args = append(args, "--fork-session")
	}

	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}

	if opts.NoSessionPersistence {
		args = append(args, "--no-session-persistence")
	}

	// --- Limits ---

	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}

	// --- Tools & Agents ---

	if len(opts.Tools) > 0 {
		args = append(args, "--tools", strings.Join(opts.Tools, ","))
	}

	if len(opts.Agents) > 0 {
		agentsJSON, err := json.Marshal(opts.Agents)
		if err != nil {
			return nil, fmt.Errorf("marshal agents: %w", err)
		}
		args = append(args, "--agents", string(agentsJSON))
	}

	if opts.DisableSlashCommands {
		args = append(args, "--disable-slash-commands")
	}

	// --- Input/Output ---

	if opts.JSONSchema != nil {
		schemaJSON, err := json.Marshal(opts.JSONSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal json schema: %w", err)
		}
		args = append(args, "--json-schema", string(schemaJSON))
	}

	if opts.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}

	if opts.InputFormat != "" {
		args = append(args, "--input-format", opts.InputFormat)
	}

	// --- Configuration ---

	if len(opts.SettingSources) > 0 {
		args = append(args, "--setting-sources", strings.Join(opts.SettingSources, ","))
	}

	if opts.Settings != "" {
		args = append(args, "--settings", opts.Settings)
	}

	for _, dir := range opts.PluginDirs {
		args = append(args, "--plugin-dir", dir)
	}

	for _, dir := range opts.AddDirs {
		args = append(args, "--add-dir", dir)
	}

	// --- MCP ---

	if mcpConfigFile != "" {
		args = append(args, "--mcp-config", mcpConfigFile)
	}

	if opts.StrictMCP {
		args = append(args, "--strict-mcp-config")
	}

	// --- Debug ---

	if opts.Debug != "" {
		args = append(args, "--debug", opts.Debug)
	}

	if opts.Chrome != nil {
		if *opts.Chrome {
			args = append(args, "--chrome")
		} else {
			args = append(args, "--no-chrome")
		}
	}

	// --- Advanced ---

	args = append(args, opts.AdditionalArgs...)

	// Prompt must be last
	args = append(args, prompt)

	return args, nil
}

// Start launches Claude CLI with the given prompt.
//
// Start can only be called once per Launcher instance. To run another
// prompt, create a new Launcher.
//
// The context controls the lifetime of the process. If the context is
// cancelled, the process is killed.
func (l *Launcher) Start(ctx context.Context, prompt string, opts LaunchOptions) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.started {
		return ErrAlreadyStarted
	}

	// Verify CLI exists
	binaryPath, err := exec.LookPath(DefaultBinary)
	if err != nil {
		return ErrCLINotFound
	}

	// Handle MCP server configuration (requires temp file)
	var mcpConfigFile string
	if len(opts.MCPServers) > 0 {
		mcpJSON, err := json.Marshal(opts.MCPServers)
		if err != nil {
			return &StartError{Err: fmt.Errorf("marshal mcp config: %w", err)}
		}

		tmpDir := os.TempDir()
		mcpFile := filepath.Join(tmpDir, fmt.Sprintf("claude-mcp-%d.json", time.Now().UnixNano()))
		if err := os.WriteFile(mcpFile, mcpJSON, 0600); err != nil {
			return &StartError{Err: fmt.Errorf("write mcp config: %w", err)}
		}
		mcpConfigFile = mcpFile
		l.tempFiles = append(l.tempFiles, mcpFile)
	}

	// Build arguments
	args, err := buildArgs(prompt, opts, mcpConfigFile)
	if err != nil {
		return &StartError{Err: err}
	}

	// Apply timeout to context if specified
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		_ = cancel // cleaned up when process exits
	}

	l.cmd = exec.CommandContext(ctx, binaryPath, args...)

	// Build environment
	l.cmd.Env = os.Environ()

	// Set API key
	if opts.APIKey != "" {
		l.cmd.Env = withEnvVar(l.cmd.Env, "ANTHROPIC_API_KEY", opts.APIKey)
	}

	// Set max thinking tokens
	if opts.MaxThinkingTokens > 0 {
		l.cmd.Env = withEnvVar(l.cmd.Env, "MAX_THINKING_TOKENS", strconv.Itoa(opts.MaxThinkingTokens))
	}

	// Merge additional environment variables
	for k, v := range opts.Env {
		l.cmd.Env = withEnvVar(l.cmd.Env, k, v)
	}

	if opts.WorkDir != "" {
		l.cmd.Dir = opts.WorkDir
	}

	l.hooks = opts.Hooks

	// Set up stdout pipe
	stdout, err := l.cmd.StdoutPipe()
	if err != nil {
		return &StartError{Err: fmt.Errorf("stdout pipe: %w", err)}
	}

	// Set up stderr pipe
	l.stderr, err = l.cmd.StderrPipe()
	if err != nil {
		return &StartError{Err: fmt.Errorf("stderr pipe: %w", err)}
	}

	// Configure scanner with large buffer for long JSON lines
	l.stdout = bufio.NewScanner(stdout)
	buf := make([]byte, 0, 256*1024)
	l.stdout.Buffer(buf, 1024*1024) // 1MB max line

	// Start the process
	l.startTime = time.Now()
	if err := l.cmd.Start(); err != nil {
		return &StartError{Err: err}
	}

	l.started = true
	l.hooks.invokeStart(l.cmd.Process.Pid)

	// Collect stderr in background
	go l.collectStderr()

	return nil
}

// collectStderr reads stderr into buffer for error reporting.
func (l *Launcher) collectStderr() {
	data, _ := io.ReadAll(l.stderr)
	l.mu.Lock()
	l.stderrBuf = data
	l.mu.Unlock()
}

// ReadMessage reads the next message from Claude's output.
//
// Returns nil, nil at EOF (Claude has finished).
// Returns nil, error on parse or I/O errors.
//
// This is a blocking call. Use a separate goroutine if you need
// concurrent processing.
func (l *Launcher) ReadMessage() (*StreamMessage, error) {
	if !l.stdout.Scan() {
		if err := l.stdout.Err(); err != nil {
			return nil, err
		}
		return nil, nil // EOF
	}

	line := l.stdout.Text()
	if line == "" {
		// Empty line, try next
		return l.ReadMessage()
	}

	var msg StreamMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		parseErr := &ParseError{Line: line, Err: err}
		l.hooks.invokeError(parseErr)
		return nil, parseErr
	}

	// Invoke hooks
	l.hooks.invokeMessage(msg)

	if text := ExtractText(&msg); text != "" {
		l.hooks.invokeText(text)
	}

	if name, input := GetToolCall(&msg); name != "" {
		l.hooks.invokeToolCall(name, input)
	}

	// Invoke metrics hook for result messages
	if msg.Type == "result" {
		m := metricsFromMessage(&msg)
		l.hooks.invokeMetrics(m)
	}

	return &msg, nil
}

// metricsFromMessage extracts SessionMetrics from a result StreamMessage.
func metricsFromMessage(msg *StreamMessage) SessionMetrics {
	m := SessionMetrics{
		CostUSD:       msg.CostUSD,
		TotalCostUSD:  msg.TotalCost,
		NumTurns:      msg.NumTurns,
		DurationMS:    msg.DurationMS,
		DurationAPIMS: msg.DurationAPIMS,
		Model:         msg.Model,
		SessionID:     msg.SessionID,
	}
	if msg.Usage != nil {
		m.InputTokens = msg.Usage.InputTokens
		m.OutputTokens = msg.Usage.OutputTokens
		m.CacheCreationInputTokens = msg.Usage.CacheCreationInputTokens
		m.CacheReadInputTokens = msg.Usage.CacheReadInputTokens
	}
	return m
}

// Wait blocks until Claude exits and returns any error.
//
// Always call Wait to ensure resources are cleaned up, even if you
// call Kill or Interrupt.
func (l *Launcher) Wait() error {
	l.mu.Lock()
	if !l.started {
		l.mu.Unlock()
		return ErrNotStarted
	}
	l.mu.Unlock()

	err := l.cmd.Wait()

	// Clean up temp files
	for _, f := range l.tempFiles {
		os.Remove(f)
	}

	// Close done channel
	l.mu.Lock()
	select {
	case <-l.done:
	default:
		close(l.done)
	}
	duration := time.Since(l.startTime)
	l.mu.Unlock()

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	l.hooks.invokeExit(exitCode, duration)

	if err != nil {
		l.mu.Lock()
		stderr := string(l.stderrBuf)
		l.mu.Unlock()

		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ExitError{Code: exitErr.ExitCode(), Stderr: stderr}
		}
		return err
	}

	return nil
}

// Interrupt sends SIGINT to Claude for graceful shutdown.
//
// Claude will attempt to finish its current operation and exit cleanly.
// Follow with Wait() to ensure the process has exited.
func (l *Launcher) Interrupt() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.started || l.cmd.Process == nil {
		return ErrNotStarted
	}

	return l.cmd.Process.Signal(syscall.SIGINT)
}

// Kill forcefully terminates Claude.
//
// Use Interrupt for graceful shutdown when possible.
// Follow with Wait() to ensure the process has exited.
func (l *Launcher) Kill() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.started || l.cmd.Process == nil {
		return ErrNotStarted
	}

	return l.cmd.Process.Kill()
}

// Done returns a channel that's closed when Claude exits.
//
// Use this for select-based waiting:
//
//	select {
//	case <-launcher.Done():
//		// Process exited
//	case <-ctx.Done():
//		launcher.Kill()
//	}
func (l *Launcher) Done() <-chan struct{} {
	return l.done
}

// PID returns the process ID of the running Claude CLI.
//
// Returns 0 if the process hasn't started or has exited.
func (l *Launcher) PID() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cmd != nil && l.cmd.Process != nil {
		return l.cmd.Process.Pid
	}
	return 0
}

// Running returns true if the Claude process is currently running.
func (l *Launcher) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.started || l.cmd == nil || l.cmd.Process == nil {
		return false
	}

	// Check if done channel is closed
	select {
	case <-l.done:
		return false
	default:
		return true
	}
}

// withEnvVar returns a copy of env with key=value set.
// If key already exists, it's replaced.
func withEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
