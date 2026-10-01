package runner

import (
	"crypto/rand"
	"fmt"
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

// newRecordUUID returns a random RFC-4122 v4 uuid, the shape real Claude Code
// stamps on every transcript record.
//
// The format matters and not merely the uniqueness: a consumer that parses the
// field, or matches a transcript's records against a uuid it was handed
// elsewhere, is entitled to a uuid rather than an arbitrary token. On a failure
// of the random source it returns the empty string, and the caller's record
// simply carries no uuid — the pre-existing behaviour, and better than a
// predictable constant that would make two sub-agents share an identity.
func newRecordUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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
