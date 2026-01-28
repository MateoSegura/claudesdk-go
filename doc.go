// Package claude provides a programmatic Go interface to the Claude CLI.
//
// This SDK offers two levels of abstraction for interacting with Claude:
//
// # Launcher (Low-Level)
//
// Launcher provides direct control over a Claude CLI subprocess with synchronous
// message reading. Use this when you need fine-grained control over the read loop:
//
//	launcher := claude.NewLauncher()
//	err := launcher.Start(ctx, "Explain quantum computing", claude.LaunchOptions{
//		SkipPermissions: true,
//	})
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
//		if text := claude.ExtractText(msg); text != "" {
//			fmt.Print(text)
//		}
//	}
//
// # Session (High-Level)
//
// Session provides a channel-based async interface with managed goroutines.
// Use this for simpler integration or when processing messages concurrently:
//
//	session, err := claude.NewSession(claude.SessionConfig{
//		WorkDir: "/path/to/project",
//		Model:   "sonnet",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := session.Run(ctx, "Write a haiku about Go"); err != nil {
//		log.Fatal(err)
//	}
//
//	for msg := range session.Messages {
//		fmt.Println(claude.ExtractText(&msg))
//	}
//
// Or use the convenience method for simple request-response:
//
//	text, err := session.CollectAll(ctx, "What is 2+2?")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(text)
//
// # Tool Extraction
//
// The SDK provides helpers for extracting structured data from Claude's responses:
//
//   - ExtractText: Get text content from any message type
//   - ExtractTodos: Parse TodoWrite tool calls
//   - ExtractBashCommand: Get commands from Bash tool calls
//   - ExtractFileAccess: Get file paths from Read/Write/Edit calls
//   - GetToolName: Identify which tool is being invoked
//
// # Hooks
//
// Optional hooks enable observability without coupling to a specific logging framework:
//
//	session, _ := claude.NewSession(claude.SessionConfig{
//		Hooks: &claude.Hooks{
//			OnMessage:  func(msg claude.StreamMessage) { log.Printf("msg: %s", msg.Type) },
//			OnToolCall: func(name string, input map[string]any) { log.Printf("tool: %s", name) },
//			OnError:    func(err error) { log.Printf("error: %v", err) },
//		},
//	})
//
// # Requirements
//
// The Claude CLI must be installed and available in PATH. Use [CLIAvailable] to check:
//
//	if !claude.CLIAvailable() {
//		log.Fatal("Claude CLI not found in PATH")
//	}
package claude
