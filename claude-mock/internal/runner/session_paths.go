package runner

import (
	"os"
	"regexp"

	"github.com/sloprail/harness-mocks/internal/session"
	coretranscript "github.com/sloprail/harness-mocks/internal/transcript"
)

const (
	// defaultConfigDir is the default CLAUDE_CONFIG_DIR for mock runs.
	// Using a fixed path (not a temp dir) means session files survive between
	// invocations and the script can read history from previous turns.
	// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions (CLAUDE_CONFIG_DIR)
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

// claudeLayout is where Claude Code keeps a session's transcript:
// <configDir>/projects/<encoded-resolved-cwd>/<session-id>.jsonl. Its project
// directory name replaces every character outside [a-zA-Z0-9] with '-' in the
// symlink-resolved cwd (a real claude run in a macOS /tmp/... directory writes
// under /private/tmp/..., the resolved form).
var claudeLayout = coretranscript.Layout{
	ProjectsDir: "projects",
	Ext:         ".jsonl",
	Encode:      func(dir string) string { return nonAlphanumRe.ReplaceAllString(dir, "-") },
}

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// sessionFilePath is where the session's transcript is, whether or not it
// exists yet.
//
// sr:provides session-transcript-file/claude
func sessionFilePath(configDir, cwd, sessionID string) string {
	return claudeLayout.FilePath(configDir, cwd, sessionID)
}

// resolveEncodingCwd is cwd with its symlinks resolved: real Claude Code
// resolves them everywhere (its PreToolUse payload's own cwd is the resolved
// form), not only in the transcript path.
func resolveEncodingCwd(cwd string) string { return coretranscript.ResolveDir(cwd) }

// newRecordUUID is the uuid real Claude Code stamps on every transcript record;
// the shared session core makes it.
func newRecordUUID() string { return session.NewID() }

// sessionFilePathIfExists is the existing transcript of sessionID, found the way
// `claude --resume <id>` finds it (in whichever project directory holds it), or
// "" when there is none.
//
// sr:provides session-resume/claude
func sessionFilePathIfExists(configDir, cwd, sessionID string) string {
	return session.Find(claudeLayout, configDir, cwd, sessionID)
}

// NewSessionID returns a fresh session id, the shape real Claude Code uses (a
// v4 uuid).
func NewSessionID() string { return newRecordUUID() }
