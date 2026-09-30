package runner

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

// sessionFilePath returns the path where the session JSONL is stored, mirroring
// the real Claude Code layout:
//
//	<configDir>/projects/<encoded-resolved-cwd>/<session-id>.jsonl
//
// The encoding rule matches Claude Code exactly: every character outside
// [a-zA-Z0-9] is replaced with '-', applied to the SYMLINK-RESOLVED cwd (see
// resolveEncodingCwd) — real Claude Code resolves symlinks before encoding (verified
// empirically: a real claude run in a macOS /tmp/... dir produces transcript paths under
// /private/tmp/..., the resolved form), so a consumer that independently derives "the
// transcript path for this cwd" via its OWN symlink-resolving logic (e.g. a10n-workspace's
// session.ResolveWorkDir + StableSessionID, or a test harness's pre-seeding of a fixture
// transcript at the resolved path) must land on the SAME encoded directory the mock itself
// writes to, or the two diverge and any consumer trusting a hook payload's `transcript_path`
// field literally can never find the file. A prior version encoded cwd directly (no symlink
// resolution), which is exactly the divergence that broke transcript resolution for any
// downstream tool run from an isolated worktree.
//
// sr:docs https://code.claude.com/docs/en/agent-sdk/sessions
func sessionFilePath(configDir, cwd, sessionID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	return filepath.Join(configDir, "projects", encoded, sessionID+".jsonl")
}

// resolveEncodingCwd symlink-resolves cwd for transcript-path ENCODING. Verified empirically
// against real claude: its PreToolUse payload's OWN `cwd` field is ALREADY the resolved form
// (e.g. `/private/tmp/...` on macOS, not `/tmp/...`) — real Claude Code resolves symlinks
// consistently everywhere, not just for transcript-path encoding. Falls back to the raw cwd on
// any resolution error (e.g. a not-yet-existent directory) rather than failing the whole path
// computation.
func resolveEncodingCwd(cwd string) string {
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}
	return cwd
}

var nonAlphanumRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

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

// sessionFilePathIfExists is the transcript of sessionID — under cwd's
// project directory, else any — or "" when there is none.
func sessionFilePathIfExists(configDir, cwd, sessionID string) string {
	if p := sessionFilePath(configDir, cwd, sessionID); fileExists(p) {
		return p
	}
	return findSessionFile(configDir, sessionID)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// findSessionFile returns the transcript of sessionID in any project directory
// under configDir, or "" — how `claude --resume <id>` finds a session begun in
// another directory. An id that is not a plain file name finds nothing; when
// more than one project directory holds the id, the most recently written
// transcript wins.
func findSessionFile(configDir, sessionID string) string {
	if sessionID == "" || strings.ContainsAny(sessionID, `/\`) || sessionID == "." || sessionID == ".." {
		return ""
	}
	projects := filepath.Join(configDir, "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		return ""
	}
	best := ""
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(projects, e.Name(), sessionID+".jsonl")
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if best == "" || fi.ModTime().UnixNano() > bestMod {
			best, bestMod = p, fi.ModTime().UnixNano()
		}
	}
	return best
}

// NewSessionID returns a fresh session id, the shape real Claude Code uses (a
// v4 uuid).
func NewSessionID() string { return newRecordUUID() }
