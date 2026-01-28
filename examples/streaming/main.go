// Example: streaming messages as they arrive
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

	prompt := "Write a short story about a robot learning to paint. Include dialogue."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	// Create session with hooks for observability
	session, err := claude.NewSession(claude.SessionConfig{
		SkipPermissions: true,
		Hooks: &claude.Hooks{
			OnStart: func(pid int) {
				fmt.Fprintf(os.Stderr, "[Started: PID %d]\n", pid)
			},
			OnExit: func(code int, duration time.Duration) {
				fmt.Fprintf(os.Stderr, "\n[Exited: code=%d duration=%s]\n", code, duration)
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

	// Start streaming
	if err := session.Run(ctx, prompt); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Process messages as they arrive
	for {
		select {
		case msg, ok := <-session.Messages:
			if !ok {
				// Channel closed, we're done
				if err := session.Err(); err != nil {
					log.Printf("Session error: %v", err)
				}
				return
			}

			// Print text as it streams
			if text := claude.ExtractText(&msg); text != "" {
				fmt.Print(text)
			}

			// Show tool usage
			if tool := claude.GetToolName(&msg); tool != "" {
				fmt.Fprintf(os.Stderr, "\n[Tool: %s]\n", tool)
			}

			// Show final metrics
			if claude.IsResult(&msg) {
				fmt.Fprintf(os.Stderr, "\n[Cost: $%.6f]\n", msg.TotalCost)
			}

		case err := <-session.Errors:
			fmt.Fprintf(os.Stderr, "[Error: %v]\n", err)

		case <-session.Done():
			return

		case <-ctx.Done():
			return
		}
	}
}
