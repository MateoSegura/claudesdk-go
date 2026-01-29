// Example: exercising multiple tools and an MCP server.
//
// This demo configures the Context7 MCP server (live documentation lookup),
// then asks Claude a prompt designed to trigger several built-in tools
// (Read, Bash, Grep) plus the MCP tool. The session hooks log every tool
// invocation in real time, and a post-run summary shows what happened.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	claude "github.com/MateoSegura/claudesdk-go"
)

func main() {
	if !claude.CLIAvailable() {
		log.Fatal("Claude CLI not found in PATH")
	}

	// ---------------------------------------------------------------
	// Prompt crafted to exercise multiple tool categories:
	//   1. Context7 MCP  – look up live library documentation
	//   2. Bash          – run a shell command
	//   3. Read / Grep   – inspect local files
	// ---------------------------------------------------------------
	prompt := `Do the following steps in order:

1. Use the Context7 MCP server to look up the latest documentation for the
   Go standard library "net/http" package — specifically how to create a
   basic HTTP server with graceful shutdown.

2. Read the go.mod file in this project to determine the module name and
   Go version.

3. Run "go version" to confirm the installed Go toolchain.

4. Based on what you learned, write a brief summary (5-8 sentences) that
   covers: the project module info, the Go version installed, and the key
   steps for creating an HTTP server with graceful shutdown according to
   the official docs you retrieved.`

	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	// Track tool usage
	toolCalls := make(map[string]int)
	filesAccessed := []string{}

	session, err := claude.NewSession(claude.SessionConfig{
		SkipPermissions: true,
		WorkDir:         ".",
		Timeout:         5 * time.Minute,
		MaxTurns:        15,

		// ---- MCP server configuration ----
		// Context7 provides live, up-to-date library documentation.
		MCPServers: map[string]claude.MCPServer{
			"context7": {
				Command: "npx",
				Args:    []string{"-y", "@upstash/context7-mcp"},
			},
		},

		Hooks: &claude.Hooks{
			OnStart: func(pid int) {
				fmt.Fprintf(os.Stderr, "[Started: PID %d]\n", pid)
			},
			OnToolCall: func(name string, input map[string]any) {
				toolCalls[name]++
				fmt.Fprintf(os.Stderr, "[Tool: %s]\n", name)
			},
			OnExit: func(code int, duration time.Duration) {
				fmt.Fprintf(os.Stderr, "[Exited: code=%d duration=%s]\n", code, duration)
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	// Handle Ctrl+C gracefully
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n[Interrupting...]\n")
		session.Interrupt()
		cancel()
	}()

	// Run and collect full result
	result, err := session.RunAndCollect(ctx, prompt)
	if err != nil {
		log.Fatalf("Session failed: %v", err)
	}

	// Analyze tool usage from messages
	for _, msg := range result.Messages {
		if files := claude.ExtractAllFileAccess(&msg); len(files) > 0 {
			filesAccessed = append(filesAccessed, files...)
		}

		if todos := claude.ExtractTodos(&msg); len(todos) > 0 {
			fmt.Fprintf(os.Stderr, "[Todos created: %d]\n", len(todos))
			for _, todo := range todos {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", todo.Content, todo.Status)
			}
		}

		if cmd := claude.ExtractBashCommand(&msg); cmd != "" {
			fmt.Fprintf(os.Stderr, "[Bash: %s]\n", truncate(cmd, 80))
		}
	}

	// Print Claude's response
	fmt.Println(result.Text)

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "\n--- Summary ---\n")
	fmt.Fprintf(os.Stderr, "Duration:  %s\n", result.Duration.Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "Cost:      $%.6f\n", result.TotalCost)
	fmt.Fprintf(os.Stderr, "Messages:  %d\n", len(result.Messages))

	if len(toolCalls) > 0 {
		fmt.Fprintf(os.Stderr, "Tools used:\n")
		for tool, count := range toolCalls {
			fmt.Fprintf(os.Stderr, "  - %-30s %dx\n", tool, count)
		}
	}

	if len(filesAccessed) > 0 {
		fmt.Fprintf(os.Stderr, "Files accessed:\n")
		for _, f := range filesAccessed {
			fmt.Fprintf(os.Stderr, "  - %s\n", f)
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
