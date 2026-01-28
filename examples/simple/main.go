// Example: simple request-response using Session.CollectAll
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
	// Check CLI availability
	if !claude.CLIAvailable() {
		log.Fatal("Claude CLI not found in PATH")
	}

	// Get prompt from args or use default
	prompt := "What is the capital of France? Reply in one sentence."
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}

	// Create session with timeout
	session, err := claude.NewSession(claude.SessionConfig{
		SkipPermissions: true,
		Timeout:         2 * time.Minute,
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	// Run and collect all output
	ctx := context.Background()
	text, err := session.CollectAll(ctx, prompt)
	if err != nil {
		log.Fatalf("Claude failed: %v", err)
	}

	fmt.Println(text)
}
