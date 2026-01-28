package claude

import (
	"errors"
	"fmt"
)

// Sentinel errors for common failure modes.
var (
	// ErrCLINotFound indicates the Claude CLI binary is not in PATH.
	ErrCLINotFound = errors.New("claude: CLI not found in PATH")

	// ErrSessionTimeout indicates the session exceeded its timeout.
	ErrSessionTimeout = errors.New("claude: session timeout exceeded")

	// ErrSessionClosed indicates an operation on a closed session.
	ErrSessionClosed = errors.New("claude: session is closed")

	// ErrAlreadyStarted indicates Start was called on an active launcher.
	ErrAlreadyStarted = errors.New("claude: launcher already started")

	// ErrNotStarted indicates an operation requiring a started launcher.
	ErrNotStarted = errors.New("claude: launcher not started")
)

// ParseError wraps JSON parsing failures with context.
type ParseError struct {
	Line string
	Err  error
}

func (e *ParseError) Error() string {
	truncated := e.Line
	if len(truncated) > 100 {
		truncated = truncated[:100] + "..."
	}
	return fmt.Sprintf("claude: parse error: %v (line: %s)", e.Err, truncated)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// ExitError wraps non-zero exit codes from the CLI.
type ExitError struct {
	Code   int
	Stderr string
}

func (e *ExitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("claude: exit code %d: %s", e.Code, e.Stderr)
	}
	return fmt.Sprintf("claude: exit code %d", e.Code)
}

// StartError wraps failures during CLI startup.
type StartError struct {
	Err error
}

func (e *StartError) Error() string {
	return fmt.Sprintf("claude: start failed: %v", e.Err)
}

func (e *StartError) Unwrap() error {
	return e.Err
}
