# claudesdk-go

A powerful, idiomatic Go SDK for programmatic access to the Claude CLI.

```go
import claude "github.com/MateoSegura/claudesdk-go"
```

## Features

- **Two-tier API**: Low-level `Launcher` for control, high-level `Session` for convenience
- **Streaming support**: Process messages as they arrive via channels
- **Tool extraction**: Helpers for extracting tool calls, todos, file access, and more
- **Observability hooks**: Optional callbacks for logging and metrics
- **Context-aware**: Full support for timeouts and cancellation
- **Zero dependencies**: Just the Go standard library

## Installation

```bash
go get github.com/MateoSegura/claudesdk-go
```

Requires the [Claude CLI](https://docs.anthropic.com/claude-code) to be installed and available in PATH.

## Quick Start

### Simple Request-Response

```go
package main

import (
    "context"
    "fmt"
    "log"

    claude "github.com/MateoSegura/claudesdk-go"
)

func main() {
    session, err := claude.NewSession(claude.SessionConfig{
        SkipPermissions: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    text, err := session.CollectAll(context.Background(), "What is 2+2?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(text)
}
```

### Streaming Messages

```go
session, _ := claude.NewSession(claude.SessionConfig{})

if err := session.Run(ctx, "Explain quantum computing"); err != nil {
    log.Fatal(err)
}

for msg := range session.Messages {
    if text := claude.ExtractText(&msg); text != "" {
        fmt.Print(text)
    }

    if tool := claude.GetToolName(&msg); tool != "" {
        fmt.Printf("\n[Using tool: %s]\n", tool)
    }
}
```

### Low-Level Control

```go
launcher := claude.NewLauncher()
err := launcher.Start(ctx, "Write a poem", claude.LaunchOptions{
    Model:           "sonnet",
    WorkDir:         "/path/to/project",
    SkipPermissions: true,
})
if err != nil {
    log.Fatal(err)
}
defer launcher.Wait()

for {
    msg, err := launcher.ReadMessage()
    if err != nil {
        log.Fatal(err)
    }
    if msg == nil {
        break // EOF
    }

    // Full control over message processing
    switch msg.Type {
    case "assistant":
        fmt.Print(claude.ExtractText(msg))
    case "result":
        fmt.Printf("\nCost: $%.4f\n", msg.TotalCost)
    }
}
```

### With Observability Hooks

```go
session, _ := claude.NewSession(claude.SessionConfig{
    Hooks: &claude.Hooks{
        OnStart: func(pid int) {
            log.Printf("Claude started: PID %d", pid)
        },
        OnToolCall: func(name string, input map[string]any) {
            log.Printf("Tool: %s", name)
        },
        OnExit: func(code int, duration time.Duration) {
            log.Printf("Claude exited: code=%d duration=%s", code, duration)
        },
    },
})
```

## API Reference

### Launcher (Low-Level)

| Method | Description |
|--------|-------------|
| `NewLauncher()` | Create a new launcher |
| `Start(ctx, prompt, opts)` | Start Claude with a prompt |
| `ReadMessage()` | Read the next message (blocking) |
| `Wait()` | Wait for Claude to exit |
| `Interrupt()` | Send SIGINT for graceful shutdown |
| `Kill()` | Force terminate |
| `Done()` | Channel closed on exit |
| `PID()` | Get process ID |
| `Running()` | Check if still running |

### Session (High-Level)

| Method | Description |
|--------|-------------|
| `NewSession(cfg)` | Create a new session |
| `Run(ctx, prompt)` | Start streaming (non-blocking) |
| `CollectAll(ctx, prompt)` | Run and return all text |
| `CollectMessages(ctx, prompt)` | Run and return all messages |
| `RunAndCollect(ctx, prompt)` | Run and return full Result |
| `Wait()` | Wait for session to end |
| `Interrupt()` / `Kill()` | Stop the session |

### Extractors

| Function | Description |
|----------|-------------|
| `ExtractText(msg)` | Get text content |
| `ExtractAllText(msg)` | Get all text blocks concatenated |
| `ExtractTodos(msg)` | Get TodoWrite items |
| `ExtractBashCommand(msg)` | Get Bash command |
| `ExtractFileAccess(msg)` | Get file path from Read/Write/Edit |
| `ExtractAllFileAccess(msg)` | Get all file paths |
| `GetToolName(msg)` | Get tool being invoked |
| `GetToolCall(msg)` | Get tool name and input |
| `GetAllToolCalls(msg)` | Get all tool calls |
| `IsResult(msg)` | Check if result message |
| `IsError(msg)` | Check if error message |
| `IsAssistant(msg)` | Check if assistant message |

### Types

```go
// StreamMessage - parsed JSON from Claude's stream output
type StreamMessage struct {
    Type       string          // "system", "assistant", "result", "error"
    Subtype    string
    SessionID  string
    Model      string
    Result     string
    TotalCost  float64
    DurationMS int64
    Message    *MessageContent
    Text       string
}

// LaunchOptions - configuration for Launcher
type LaunchOptions struct {
    SkipPermissions bool
    WorkDir         string
    Model           string
    SystemPrompt    string
    MaxTurns        int
    Timeout         time.Duration
    Verbose         bool
    Hooks           *Hooks
}

// SessionConfig - configuration for Session
type SessionConfig struct {
    ID              string
    WorkDir         string
    Model           string
    SystemPrompt    string
    SkipPermissions bool
    MaxTurns        int
    Timeout         time.Duration
    ChannelBuffer   int
    Hooks           *Hooks
}

// Hooks - optional observability callbacks
type Hooks struct {
    OnMessage  func(StreamMessage)
    OnText     func(string)
    OnToolCall func(name string, input map[string]any)
    OnError    func(error)
    OnStart    func(pid int)
    OnExit     func(code int, duration time.Duration)
}
```

### Errors

```go
var ErrCLINotFound    = errors.New("claude: CLI not found in PATH")
var ErrSessionTimeout = errors.New("claude: session timeout exceeded")
var ErrSessionClosed  = errors.New("claude: session is closed")
var ErrAlreadyStarted = errors.New("claude: launcher already started")
var ErrNotStarted     = errors.New("claude: launcher not started")

type ParseError struct { Line string; Err error }
type ExitError struct { Code int; Stderr string }
type StartError struct { Err error }
```

## License

MIT
