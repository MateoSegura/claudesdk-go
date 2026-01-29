// Example: exercising multiple tools and an MCP server with real-time output.
//
// This demo configures the Context7 MCP server (live documentation lookup),
// then asks Claude a prompt designed to trigger several built-in tools
// (Read, Bash, Grep) plus the MCP tool. Real-time streaming output shows
// text as it arrives and tool invocations in color.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	claude "github.com/MateoSegura/claudesdk-go"
)

// ANSI color codes
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	blue    = "\033[34m"
	magenta = "\033[35m"
	cyan    = "\033[36m"
	white   = "\033[37m"
)

// toolColor returns a consistent color for a tool name.
func toolColor(name string) string {
	switch {
	case name == "Bash":
		return green
	case name == "Read":
		return cyan
	case name == "Write" || name == "Edit":
		return yellow
	case name == "Grep" || name == "Glob":
		return magenta
	case strings.HasPrefix(name, "mcp__"):
		return blue
	default:
		return white
	}
}

func main() {
	if !claude.CLIAvailable() {
		log.Fatal("Claude CLI not found in PATH")
	}

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

	// Track stats
	toolCalls := make(map[string]int)
	filesAccessed := []string{}
	startTime := time.Now()

	// Header
	fmt.Fprintf(os.Stderr, "\n%s%s claudesdk-go tools example %s\n", bold, cyan, reset)
	fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", dim, reset)

	session, err := claude.NewSession(claude.SessionConfig{
		SkipPermissions: true,
		WorkDir:         ".",
		Timeout:         5 * time.Minute,
		MaxTurns:        15,

		MCPServers: map[string]claude.MCPServer{
			"context7": {
				Command: "npx",
				Args:    []string{"-y", "@upstash/context7-mcp"},
			},
		},

		Hooks: &claude.Hooks{
			OnStart: func(pid int) {
				fmt.Fprintf(os.Stderr, "%s%s PID %d %s\n\n", dim, green, pid, reset)
			},
			OnToolCall: func(name string, input map[string]any) {
				toolCalls[name]++
				color := toolColor(name)

				// Clean up MCP tool names for display
				displayName := name
				if strings.HasPrefix(name, "mcp__") {
					parts := strings.Split(name, "__")
					if len(parts) >= 3 {
						displayName = fmt.Sprintf("%s → %s", parts[1], parts[2])
					}
				}

				// Show a one-line summary depending on tool type
				detail := ""
				switch name {
				case "Bash":
					if cmd, ok := input["command"].(string); ok {
						cmd = strings.Split(cmd, "\n")[0]
						if len(cmd) > 60 {
							cmd = cmd[:57] + "..."
						}
						detail = fmt.Sprintf(" %s$ %s%s", dim, cmd, reset)
					}
				case "Read":
					if fp, ok := input["file_path"].(string); ok {
						detail = fmt.Sprintf(" %s%s%s", dim, fp, reset)
						filesAccessed = append(filesAccessed, fp)
					}
				case "Write", "Edit":
					if fp, ok := input["file_path"].(string); ok {
						detail = fmt.Sprintf(" %s%s%s", dim, fp, reset)
						filesAccessed = append(filesAccessed, fp)
					}
				case "Grep", "Glob":
					if p, ok := input["pattern"].(string); ok {
						detail = fmt.Sprintf(" %s%s%s", dim, p, reset)
					}
				default:
					if strings.HasPrefix(name, "mcp__") {
						if q, ok := input["query"].(string); ok {
							if len(q) > 50 {
								q = q[:47] + "..."
							}
							detail = fmt.Sprintf(" %s\"%s\"%s", dim, q, reset)
						}
						if lib, ok := input["libraryName"].(string); ok {
							detail = fmt.Sprintf(" %s%s%s", dim, lib, reset)
						}
					}
				}

				fmt.Fprintf(os.Stderr, "  %s%s● %s%s%s\n", color, bold, displayName, reset, detail)
			},
			OnExit: func(code int, duration time.Duration) {
				fmt.Fprintf(os.Stderr, "\n")
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	// Handle Ctrl+C
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\n%s[Interrupting...]%s\n", yellow, reset)
		session.Interrupt()
		cancel()
	}()

	// Start streaming
	if err := session.Run(ctx, prompt); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Print text in real time, tools via hooks above
	var fullText strings.Builder
	for {
		select {
		case msg, ok := <-session.Messages:
			if !ok {
				goto done
			}

			if msg.Type == "assistant" {
				if text := claude.ExtractText(&msg); text != "" {
					fmt.Print(text)
					fullText.WriteString(text)
				}
			}

			// Capture result metrics
			if claude.IsResult(&msg) {
				// Summary banner
				elapsed := time.Since(startTime)
				fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", dim, reset)
				fmt.Fprintf(os.Stderr, "%s%s Summary%s\n", bold, cyan, reset)
				fmt.Fprintf(os.Stderr, "  Duration   %s%s%s\n", white, elapsed.Round(time.Millisecond), reset)
				fmt.Fprintf(os.Stderr, "  Cost       %s$%.6f%s\n", green, msg.TotalCost, reset)

				if len(toolCalls) > 0 {
					fmt.Fprintf(os.Stderr, "  Tools\n")
					for tool, count := range toolCalls {
						color := toolColor(tool)
						displayTool := tool
						if strings.HasPrefix(tool, "mcp__") {
							parts := strings.Split(tool, "__")
							if len(parts) >= 3 {
								displayTool = fmt.Sprintf("%s → %s", parts[1], parts[2])
							}
						}
						fmt.Fprintf(os.Stderr, "    %s●%s %-35s %s%dx%s\n", color, reset, displayTool, dim, count, reset)
					}
				}

				if len(filesAccessed) > 0 {
					fmt.Fprintf(os.Stderr, "  Files\n")
					for _, f := range filesAccessed {
						fmt.Fprintf(os.Stderr, "    %s→%s %s\n", cyan, reset, f)
					}
				}

				fmt.Fprintf(os.Stderr, "%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", dim, reset)
			}

		case <-session.Errors:
			// non-fatal, continue

		case <-session.Done():
			goto done

		case <-ctx.Done():
			goto done
		}
	}

done:
	if err := session.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "%sSession error: %v%s\n", red, err, reset)
	}
}
