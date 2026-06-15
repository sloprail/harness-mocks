package runner

import (
	"os"
	"path/filepath"
	"regexp"
)

const (
	// defaultConfigDir is the default CLAUDE_CONFIG_DIR for mock runs.
	// Using a fixed path (not a temp dir) means session files survive between
	// invocations and the script can read history from previous turns.
	// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
	defaultConfigDir = "/tmp/a10n-mock"
)

// resolveConfigDir returns the effective config dir: explicit arg wins, then
// CLAUDE_CONFIG_DIR env var, then defaultConfigDir.
func resolveConfigDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return defaultConfigDir
}

// sessionFilePath returns the path where the session JSONL is stored, mirroring
// the real Claude Code layout:
//
//	<configDir>/projects/<encoded-cwd>/<session-id>.jsonl
//
// The encoding rule matches Claude Code exactly: every character outside
// [a-zA-Z0-9] is replaced with '-'.
//
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions
func sessionFilePath(configDir, cwd, sessionID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(cwd, "-")
	return filepath.Join(configDir, "projects", encoded, sessionID+".jsonl")
}

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// openSessionFile creates (or appends to) the session JSONL file and returns
// the open file handle. The caller is responsible for closing it.
func openSessionFile(configDir, cwd, sessionID string) (*os.File, error) {
	path := sessionFilePath(configDir, cwd, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// appendToSession writes one JSONL line to the session file.
// Errors are silently ignored — session persistence is best-effort; the mock
// must not fail because of a session write error.
func appendToSession(f *os.File, line []byte) {
	if f == nil {
		return
	}
	f.Write(line)      //nolint:errcheck
	f.Write([]byte{'\n'}) //nolint:errcheck
}
