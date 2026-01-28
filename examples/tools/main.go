// Example: reacting to tool calls
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	claude "github.com/MateoSegura/claudesdk-go"
)

func main() {
	if !claude.CLIAvailable() {
		log.Fatal("Claude CLI not found in PATH")
	}

	// A prompt that will trigger tool usage
	prompt := "List the files in the current directory and tell me what you see."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	// Track tool usage
	toolCalls := make(map[string]int)
	filesAccessed := []string{}

	session, err := claude.NewSession(claude.SessionConfig{
		SkipPermissions: true,
		WorkDir:         ".", // Current directory
		Timeout:         5 * time.Minute,
		Hooks: &claude.Hooks{
			OnToolCall: func(name string, input map[string]any) {
				toolCalls[name]++
				fmt.Fprintf(os.Stderr, "[Tool: %s]\n", name)
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	ctx := context.Background()
	result, err := session.RunAndCollect(ctx, prompt)
	if err != nil {
		log.Fatalf("Session failed: %v", err)
	}

	// Analyze tool usage from messages
	for _, msg := range result.Messages {
		// Track file access
		if files := claude.ExtractAllFileAccess(&msg); len(files) > 0 {
			filesAccessed = append(filesAccessed, files...)
		}

		// Track todos created
		if todos := claude.ExtractTodos(&msg); len(todos) > 0 {
			fmt.Fprintf(os.Stderr, "[Todos created: %d]\n", len(todos))
			for _, todo := range todos {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", todo.Content, todo.Status)
			}
		}

		// Track bash commands
		if cmd := claude.ExtractBashCommand(&msg); cmd != "" {
			fmt.Fprintf(os.Stderr, "[Bash: %s]\n", truncate(cmd, 60))
		}
	}

	// Print response
	fmt.Println("\n" + result.Text)

	// Print summary
	fmt.Fprintf(os.Stderr, "\n--- Summary ---\n")
	fmt.Fprintf(os.Stderr, "Duration: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Fprintf(os.Stderr, "Cost: $%.6f\n", result.TotalCost)
	fmt.Fprintf(os.Stderr, "Messages: %d\n", len(result.Messages))

	if len(toolCalls) > 0 {
		fmt.Fprintf(os.Stderr, "Tools used:\n")
		for tool, count := range toolCalls {
			fmt.Fprintf(os.Stderr, "  - %s: %d\n", tool, count)
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
