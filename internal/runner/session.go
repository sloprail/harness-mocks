package runner

import (
	"crypto/rand"
	"encoding/json"
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
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions
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

// openSessionFile creates (or appends to) the session JSONL file and returns
// the open file handle. The caller is responsible for closing it.
func openSessionFile(configDir, cwd, sessionID string) (*os.File, error) {
	path := sessionFilePath(configDir, cwd, sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// seedRootPromptTranscript writes the root agent's `-p` prompt as the FIRST user record
// of the session transcript — mirroring real Claude Code, whose transcript opens with the
// user prompt. A plugin's Stop hook reads the transcript's first user message (e.g. to
// harvest a10n:// links into check contexts), so the mock must persist the prompt there;
// without it the root transcript would contain only assistant/result records and that hook
// path could never be exercised. Best-effort + idempotent: only writes when the session
// file is still EMPTY (so a resume, whose transcript is already seeded, is left untouched).
func seedRootPromptTranscript(f *os.File, sessionID, cwd, prompt string) {
	if f == nil || prompt == "" {
		return
	}
	if fi, err := f.Stat(); err != nil || fi.Size() > 0 {
		return // already has content (resume / re-entry) — don't duplicate the prompt
	}
	rec := map[string]any{
		"type":      "user",
		"sessionId": sessionID,
		"cwd":       cwd,
		"message":   map[string]any{"role": "user", "content": prompt},
	}
	if line, err := json.Marshal(rec); err == nil {
		appendToSession(f, line)
	}
}

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

// appendToSession writes one JSONL line to the session file.
// Errors are silently ignored — session persistence is best-effort; the mock
// must not fail because of a session write error.
func appendToSession(f *os.File, line []byte) {
	if f == nil {
		return
	}
	f.Write(line)         //nolint:errcheck
	f.Write([]byte{'\n'}) //nolint:errcheck
}

// subagentTranscriptPath returns the path of a subagent's sidechain transcript.
//
// The <configDir>/projects/<encoded-resolved-cwd>/ prefix and the non-alphanumeric→'-'
// encoding are documented; the symlink resolution before encoding matches
// sessionFilePath's own (see resolveEncodingCwd's doc — real Claude Code resolves
// symlinks consistently, and any consumer resolving this SAME path independently — e.g.
// a10n-workspace's StableSessionID/ResolveWorkDir, driven by the sub-agent's OWN
// os.Getwd() (already resolved) once it walks back up to the parent project dir — must
// land on the identical encoded directory, or transcript resolution silently fails for
// any tool trusting a hook payload's `transcript_path` field literally). The
// <parentSessionID>/subagents/agent-<id>.jsonl suffix is NOT documented — it is
// replicated from the REAL claude CLI's on-disk layout (observed at
// ~/.claude/projects/<proj>/<root>/subagents/agent-<hash>.jsonl + .meta.json during the
// hook PoC), so the mock's transcript_path matches what real subagent hooks receive.
//
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (projects/<encoded-cwd> prefix + CLAUDE_CONFIG_DIR)
func subagentTranscriptPath(configDir, cwd, parentSessionID, agentID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(resolveEncodingCwd(cwd), "-")
	return filepath.Join(configDir, "projects", encoded, parentSessionID, "subagents", "agent-"+agentID+".jsonl")
}

// seedSubagentTranscript writes prompt as the first user record of the subagent's
// sidechain transcript and writes the .meta.json sidecar (agentType/worktreePath/
// description/toolUseId, as the real CLI does), returning the transcript path.
//
// The real Claude subagent transcript's first record IS the dispatch prompt, and
// the SubagentStart hook receives this path as transcript_path — so a hook can
// read the prompt out of it (e.g. to recover an embedded `--task-id`). Returns the
// path even on a best-effort write failure so the hook still gets a target.
//
// parentCwd keys the transcript's PROJECT DIR: real claude nests the subagent's
// sidechain under the PARENT session's transcript directory (…/projects/<encoded
// parent cwd>/<session>/subagents/…), regardless of the subagent's own worktree.
// subCwd is what the subagent's env reports (its isolated worktree under
// isolation="worktree", else == parentCwd); it is what lands in the record's `cwd`
// and the meta `worktreePath`, so a hook reading the transcript sees the isolated cwd.
//
// toolUseId is the id of the Agent/Task tool_use in the PARENT transcript that
// spawned this subagent. Real Claude Code records it in the .meta.json sidecar,
// and a consumer deriving the subagent's parentPath (which parent tool_use this
// sidechain answers) reads it — so the mock must stamp the REAL id here rather
// than the empty string a prior version hardcoded. Empty only when the spawning
// tool_use carried no id (e.g. a scenario that omitted one).
//
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions
func seedSubagentTranscript(configDir, parentCwd, subCwd, parentSessionID, agentID, agentType, toolUseID, prompt string) string {
	path := subagentTranscriptPath(configDir, parentCwd, parentSessionID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path
	}
	rec := map[string]any{
		"type":      "user",
		"sessionId": parentSessionID,
		// uuid + an explicit null parentUuid make this record the transcript's
		// ORIGIN, which is what a consumer keys the sub-agent's identity by.
		// Verified against a real Claude sub-agent transcript, whose first record
		// carries a uuid with parentUuid null and every later record chains from
		// it. The mock omitted the uuid, so every record in the file looked
		// parentless-but-anonymous and no origin could be found at all — a
		// consumer resolving a stable session id from this path got "every entry
		// has a parent" and had to stand down, which read from the outside as a
		// sub-agent whose cycle could not be judged.
		"uuid":        newRecordUUID(),
		"parentUuid":  nil,
		"isSidechain": true,
		// agentId mirrors the REAL Claude Code subagent transcript: every record
		// in subagents/agent-<AGENTID>.jsonl carries a top-level agentId equal to
		// the file's agent id. The parallel-subagent task-id attribution path
		// (locate-task-id --agent-id) reads this field, so the mock must seed it
		// for that deterministic path to be exercised in mock-based harnesses.
		"agentId": agentID,
		"cwd":     subCwd,
		"message": map[string]any{"role": "user", "content": prompt},
	}
	if line, err := json.Marshal(rec); err == nil {
		if f, ferr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			appendToSession(f, line)
			f.Close()
		}
	}
	// Best-effort .meta.json sidecar (mirrors the real layout; not all hooks read it).
	// worktreePath is the subagent's isolated cwd when it differs from the parent
	// (isolation="worktree"), matching the real CLI's sidecar.
	worktreePath := ""
	if subCwd != parentCwd {
		worktreePath = subCwd
	}
	meta := map[string]any{"agentType": agentType, "worktreePath": worktreePath, "description": "", "toolUseId": toolUseID}
	if mb, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".meta.json", mb, 0o644)
	}
	return path
}
