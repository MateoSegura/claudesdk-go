package claude

import (
	"os/exec"
	"strings"
)

// Version is the SDK version.
const Version = "0.2.0"

// DefaultBinary is the expected Claude CLI binary name.
const DefaultBinary = "claude"

// CLIAvailable checks if the Claude CLI is available in PATH.
//
// This is a convenience function for startup checks. Returns true
// if the claude binary is found and executable.
func CLIAvailable() bool {
	_, err := exec.LookPath(DefaultBinary)
	return err == nil
}

// CLIVersion returns the Claude CLI version string.
//
// Returns empty string and error if the CLI is not available
// or version cannot be determined.
func CLIVersion() (string, error) {
	path, err := exec.LookPath(DefaultBinary)
	if err != nil {
		return "", ErrCLINotFound
	}

	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// MustCLIAvailable panics if the Claude CLI is not available.
//
// Use in init() or main() for fail-fast behavior:
//
//	func init() {
//		claude.MustCLIAvailable()
//	}
func MustCLIAvailable() {
	if !CLIAvailable() {
		panic(ErrCLINotFound)
	}
}
