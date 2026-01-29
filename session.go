package claude

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Session provides a high-level, channel-based interface to Claude CLI.
//
// Session manages goroutines internally and provides channels for consuming
// messages asynchronously. It's built on top of Launcher and is the recommended
// API for most use cases.
//
// Example:
//
//	session, err := claude.NewSession(claude.SessionConfig{
//		LaunchOptions: claude.LaunchOptions{
//			Model:           "sonnet",
//			SkipPermissions: true,
//		},
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := session.Run(ctx, "Explain this codebase"); err != nil {
//		log.Fatal(err)
//	}
//
//	for msg := range session.Messages {
//		fmt.Println(claude.ExtractText(&msg))
//	}
type Session struct {
	// ID is the session identifier (from config or auto-generated).
	ID string

	// Messages receives all parsed StreamMessages.
	// Closed when the session ends.
	Messages chan StreamMessage

	// Text receives extracted text content.
	// Closed when the session ends.
	Text chan string

	// Errors receives non-fatal errors (parse errors, etc.).
	// Closed when the session ends.
	Errors chan error

	launcher *Launcher
	config   SessionConfig

	mu      sync.Mutex
	closed  bool
	done    chan struct{}
	err     error
	metrics SessionMetrics
}

// NewSession creates a new Session with the given configuration.
//
// The session is not started until Run is called.
func NewSession(cfg SessionConfig) (*Session, error) {
	bufSize := cfg.ChannelBuffer
	if bufSize <= 0 {
		bufSize = 100
	}

	id := cfg.ID
	if id == "" {
		id = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	return &Session{
		ID:       id,
		Messages: make(chan StreamMessage, bufSize),
		Text:     make(chan string, bufSize),
		Errors:   make(chan error, 10),
		config:   cfg,
		done:     make(chan struct{}),
	}, nil
}

// Run executes a prompt and streams results to channels.
//
// Run is non-blocking; it starts the Claude process and returns immediately.
// Consume the Messages, Text, or Errors channels to receive output.
// When Claude exits, all channels are closed.
//
// Run can only be called once per Session. Create a new Session for
// additional prompts.
func (s *Session) Run(ctx context.Context, prompt string) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSessionClosed
	}
	s.mu.Unlock()

	s.launcher = NewLauncher()

	if err := s.launcher.Start(ctx, prompt, s.config.LaunchOptions); err != nil {
		s.close()
		return err
	}

	// Start reading goroutine
	go s.readLoop()

	// Start waiter goroutine
	go s.waitLoop()

	return nil
}

// readLoop reads messages from the launcher and dispatches to channels.
func (s *Session) readLoop() {
	defer s.close()

	for {
		msg, err := s.launcher.ReadMessage()
		if err != nil {
			s.sendError(err)
			continue
		}
		if msg == nil {
			break // EOF
		}

		s.sendMessage(*msg)

		// Only send text from assistant messages to avoid duplicates.
		// Result messages repeat the same text content.
		if msg.Type == "assistant" {
			if text := ExtractText(msg); text != "" {
				s.sendText(text)
			}
		}

		// Update metrics from result messages
		if msg.Type == "result" {
			m := metricsFromMessage(msg)
			s.mu.Lock()
			s.metrics = m
			s.mu.Unlock()
		}

		// Capture session info from system init message
		if msg.Type == "system" && msg.Subtype == "init" && msg.SessionID != "" {
			s.mu.Lock()
			s.metrics.SessionID = msg.SessionID
			s.metrics.Model = msg.Model
			s.mu.Unlock()
		}
	}
}

// waitLoop waits for the launcher to exit and captures the error.
func (s *Session) waitLoop() {
	err := s.launcher.Wait()

	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

// sendMessage sends a message to the Messages channel without blocking.
func (s *Session) sendMessage(msg StreamMessage) {
	select {
	case s.Messages <- msg:
	default:
		// Buffer full, drop message
		s.sendError(fmt.Errorf("message channel buffer full, dropping message"))
	}
}

// sendText sends text to the Text channel without blocking.
func (s *Session) sendText(text string) {
	select {
	case s.Text <- text:
	default:
		// Buffer full, drop
	}
}

// sendError sends an error to the Errors channel without blocking.
func (s *Session) sendError(err error) {
	select {
	case s.Errors <- err:
	default:
		// Buffer full, drop
	}
}

// close closes all channels once.
func (s *Session) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	close(s.done)
	close(s.Messages)
	close(s.Text)
	close(s.Errors)
}

// Done returns a channel that's closed when the session ends.
func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Err returns the error from the session, if any.
//
// Call after Done() is closed to get the final error.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Wait blocks until the session ends and returns any error.
func (s *Session) Wait() error {
	<-s.done
	return s.Err()
}

// CurrentMetrics returns the latest accumulated session metrics.
//
// Safe to call from any goroutine at any time. Returns zero-value
// SessionMetrics until the result message arrives.
//
// For async metrics delivery, use the OnMetrics hook instead.
func (s *Session) CurrentMetrics() SessionMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

// Interrupt sends SIGINT to Claude for graceful shutdown.
func (s *Session) Interrupt() error {
	if s.launcher == nil {
		return ErrNotStarted
	}
	return s.launcher.Interrupt()
}

// Kill forcefully terminates Claude.
func (s *Session) Kill() error {
	if s.launcher == nil {
		return ErrNotStarted
	}
	return s.launcher.Kill()
}

// CollectAll runs a prompt and returns all text output.
//
// This is a convenience method for simple request-response patterns.
// It blocks until Claude finishes or the context is cancelled.
//
// Example:
//
//	session, _ := claude.NewSession(claude.SessionConfig{})
//	text, err := session.CollectAll(ctx, "What is 2+2?")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Println(text) // "2+2 = 4"
func (s *Session) CollectAll(ctx context.Context, prompt string) (string, error) {
	if err := s.Run(ctx, prompt); err != nil {
		return "", err
	}

	var builder strings.Builder

	for {
		select {
		case text, ok := <-s.Text:
			if !ok {
				return builder.String(), s.Err()
			}
			builder.WriteString(text)

		case err, ok := <-s.Errors:
			if ok && err != nil {
				_ = err
			}

		case <-s.Done():
			// Drain remaining text
			for text := range s.Text {
				builder.WriteString(text)
			}
			return builder.String(), s.Err()

		case <-ctx.Done():
			s.Kill()
			return builder.String(), ctx.Err()
		}
	}
}

// CollectMessages runs a prompt and returns all messages.
//
// Similar to CollectAll but returns the full StreamMessage slice
// for access to metadata, tool calls, and other structured data.
func (s *Session) CollectMessages(ctx context.Context, prompt string) ([]StreamMessage, error) {
	if err := s.Run(ctx, prompt); err != nil {
		return nil, err
	}

	var messages []StreamMessage

	for {
		select {
		case msg, ok := <-s.Messages:
			if !ok {
				return messages, s.Err()
			}
			messages = append(messages, msg)

		case <-s.Errors:
			// Ignore non-fatal errors in collect mode

		case <-s.Done():
			// Drain remaining messages
			for msg := range s.Messages {
				messages = append(messages, msg)
			}
			return messages, s.Err()

		case <-ctx.Done():
			s.Kill()
			return messages, ctx.Err()
		}
	}
}

// Result holds the final metrics from a completed session.
type Result struct {
	// Text is the collected text output.
	Text string

	// Messages is the full list of messages.
	Messages []StreamMessage

	// TotalCost is the total cost in USD.
	TotalCost float64

	// CostUSD is the cost for this specific invocation.
	CostUSD float64

	// Duration is the total runtime.
	Duration time.Duration

	// DurationAPI is the API-only duration (excludes tool execution time).
	DurationAPI time.Duration

	// Model is the model that was used.
	Model string

	// SessionID is Claude's session identifier.
	SessionID string

	// NumTurns is the number of agentic turns completed.
	NumTurns int

	// Usage contains token consumption data. Nil if not available.
	Usage *Usage

	// StructuredOutput contains validated JSON when --json-schema was used.
	StructuredOutput any

	// Metrics is the full session metrics snapshot.
	Metrics SessionMetrics
}

// RunAndCollect runs a prompt and returns a comprehensive Result.
//
// This is the most complete collection method, returning text, messages,
// and all available metadata including cost, tokens, and structured output.
func (s *Session) RunAndCollect(ctx context.Context, prompt string) (*Result, error) {
	if err := s.Run(ctx, prompt); err != nil {
		return nil, err
	}

	result := &Result{}
	var textBuilder strings.Builder
	startTime := time.Now()

	for {
		select {
		case msg, ok := <-s.Messages:
			if !ok {
				result.Duration = time.Since(startTime)
				result.Text = textBuilder.String()
				result.Metrics = s.CurrentMetrics()
				return result, s.Err()
			}

			result.Messages = append(result.Messages, msg)

			// Only collect text from assistant messages to avoid duplicates.
			if msg.Type == "assistant" {
				if text := ExtractText(&msg); text != "" {
					textBuilder.WriteString(text)
				}
			}

			// Capture metadata from result message
			if msg.Type == "result" {
				result.TotalCost = msg.TotalCost
				result.CostUSD = msg.CostUSD
				if msg.SessionID != "" {
					result.SessionID = msg.SessionID
				}
				if msg.Model != "" {
					result.Model = msg.Model
				}
				result.NumTurns = msg.NumTurns
				result.Usage = msg.Usage
				result.StructuredOutput = msg.StructuredOutput
				if msg.DurationAPIMS > 0 {
					result.DurationAPI = time.Duration(msg.DurationAPIMS) * time.Millisecond
				}
			}

			// Capture session info from system init message
			if msg.Type == "system" && msg.Subtype == "init" {
				if msg.SessionID != "" {
					result.SessionID = msg.SessionID
				}
				if msg.Model != "" {
					result.Model = msg.Model
				}
			}

		case <-s.Errors:
			// Ignore non-fatal errors

		case <-s.Done():
			// Drain remaining
			for msg := range s.Messages {
				result.Messages = append(result.Messages, msg)
				if msg.Type == "assistant" {
					if text := ExtractText(&msg); text != "" {
						textBuilder.WriteString(text)
					}
				}
				if msg.Type == "result" {
					result.TotalCost = msg.TotalCost
					result.CostUSD = msg.CostUSD
					result.NumTurns = msg.NumTurns
					result.Usage = msg.Usage
					result.StructuredOutput = msg.StructuredOutput
					if msg.DurationAPIMS > 0 {
						result.DurationAPI = time.Duration(msg.DurationAPIMS) * time.Millisecond
					}
				}
			}
			result.Duration = time.Since(startTime)
			result.Text = textBuilder.String()
			result.Metrics = s.CurrentMetrics()
			return result, s.Err()

		case <-ctx.Done():
			s.Kill()
			result.Duration = time.Since(startTime)
			result.Text = textBuilder.String()
			result.Metrics = s.CurrentMetrics()
			return result, ctx.Err()
		}
	}
}
