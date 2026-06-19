package runner

import (
	"encoding/json"
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
	f.Write(line)         //nolint:errcheck
	f.Write([]byte{'\n'}) //nolint:errcheck
}

// subagentTranscriptPath returns the path of a subagent's sidechain transcript.
//
// The <configDir>/projects/<encoded-cwd>/ prefix and the non-alphanumeric→'-'
// cwd encoding are documented. The <parentSessionID>/subagents/agent-<id>.jsonl
// suffix is NOT documented — it is replicated from the REAL claude CLI's
// on-disk layout (observed at ~/.claude/projects/<proj>/<root>/subagents/
// agent-<hash>.jsonl + .meta.json during the hook PoC), so the mock's
// transcript_path matches what real subagent hooks receive.
//
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions (projects/<encoded-cwd> prefix + CLAUDE_CONFIG_DIR)
func subagentTranscriptPath(configDir, cwd, parentSessionID, agentID string) string {
	encoded := nonAlphanumRe.ReplaceAllString(cwd, "-")
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
// a10n:docs https://code.claude.com/docs/en/agent-sdk/sessions
func seedSubagentTranscript(configDir, cwd, parentSessionID, agentID, agentType, prompt string) string {
	path := subagentTranscriptPath(configDir, cwd, parentSessionID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path
	}
	rec := map[string]any{
		"type":        "user",
		"sessionId":   parentSessionID,
		"isSidechain": true,
		// agentId mirrors the REAL Claude Code subagent transcript: every record
		// in subagents/agent-<AGENTID>.jsonl carries a top-level agentId equal to
		// the file's agent id. The parallel-subagent task-id attribution path
		// (locate-task-id --agent-id) reads this field, so the mock must seed it
		// for that deterministic path to be exercised in mock-based harnesses.
		"agentId":     agentID,
		"cwd":         cwd,
		"message":     map[string]any{"role": "user", "content": prompt},
	}
	if line, err := json.Marshal(rec); err == nil {
		if f, ferr := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			appendToSession(f, line)
			f.Close()
		}
	}
	// Best-effort .meta.json sidecar (mirrors the real layout; not all hooks read it).
	meta := map[string]any{"agentType": agentType, "worktreePath": "", "description": "", "toolUseId": ""}
	if mb, err := json.MarshalIndent(meta, "", "  "); err == nil {
		_ = os.WriteFile(strings.TrimSuffix(path, ".jsonl")+".meta.json", mb, 0o644)
	}
	return path
}
