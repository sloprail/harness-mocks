package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/session"
)

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

// openRunTranscript decides which file a run writes and which path its hooks
// are told about:
//
//   - a nested SUB-AGENT run writes the sub-agent's own sidechain file and
//     reports the PARENT's transcript_path (the sub-agent is named by agent_id);
//   - a FORK (--resume <old> --fork-session) writes a new file under this run's
//     own session id, seeded by forkTranscript;
//   - a RESUME writes the session's existing file — found in whichever project
//     directory holds it, when that is not this run's — and reports the path
//     under this run's own directory, as real Claude Code does (see transcript);
//   - anything else writes, and reports, <projects>/<encoded cwd>/<id>.jsonl.
//
// sr:provides session-resume/claude
func openRunTranscript(cfg Config) (*transcript, error) {
	stamp := newRecordStamp(cfg.SessionID, cfg.Cwd)
	if cfg.SidechainPath != "" {
		stamp.IsSidechain = true
		stamp.AgentID = cfg.AgentID
		return openTranscript(cfg.SidechainPath, cfg.ParentTranscriptPath, stamp)
	}
	here := sessionFilePath(cfg.ConfigDir, cfg.Cwd, cfg.SessionID)
	if cfg.ForkFrom != "" {
		if err := forkTranscript(cfg.ConfigDir, cfg.Cwd, cfg.ForkFrom, here, cfg.SessionID); err != nil {
			return nil, err
		}
		return openTranscript(here, here, stamp)
	}
	if cfg.IsResume {
		if r, err := session.Resume(claudeLayout, cfg.ConfigDir, cfg.Cwd, cfg.SessionID); err == nil {
			return openTranscript(r.Path, r.Reported, stamp)
		}
	}
	tr, err := openTranscript(here, here, stamp)
	if tr != nil {
		tr.name = cfg.Name
	}
	return tr, err
}

// writeRootPrompt writes a FRESH session's prompt as the human turn it is,
// chained after whatever the session has written so far — which, if a
// SessionStart hook printed anything, is that hook's attachment. Its uuid is
// the deterministic `e2e-root-<session>`. Like a real `claude -p` prompt it is
// marked promptSource/turnOrigin "sdk".
//
// A caller that pre-seeded the transcript with that same record before the
// mock ran (the older sloprail harness did) is honoured rather than
// duplicated: the record is left where it is and the preamble put ahead of it.
func writeRootPrompt(tr *transcript, sessionID, cwd, prompt string) {
	if prompt == "" {
		return
	}
	rootID := "e2e-root-" + sessionID
	if tr.exists() && fileHasUUID(tr.path, rootID) {
		seedPreamble(tr.file(), sessionID, tr.name)
		return
	}
	rec := map[string]any{
		"type":    "user",
		"uuid":    rootID,
		"cwd":     cwd,
		"message": map[string]any{"role": "user", "content": prompt},
	}
	markPrompt(rec)
	tr.persistMap(rec)
}

// appendResumePrompt writes a RESUME's prompt as the next human turn, chained
// to the last record on disk. A re-entry that already wrote this prompt as the
// last human turn is not written twice.
func appendResumePrompt(tr *transcript, sessionID, prompt string) {
	if prompt == "" {
		return
	}
	if _, last := scanTranscriptTail(tr.path); last == prompt {
		return
	}
	rec := map[string]any{
		"type":    "user",
		"uuid":    newRecordUUID(),
		"message": map[string]any{"role": "user", "content": prompt},
	}
	markPrompt(rec)
	tr.persistMap(rec)
}

// fileHasUUID reports whether any record in path has uuid as its own.
func fileHasUUID(path, uuid string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, uuid) {
			continue
		}
		var rec struct {
			UUID string `json:"uuid"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.UUID == uuid {
			return true
		}
	}
	return false
}
