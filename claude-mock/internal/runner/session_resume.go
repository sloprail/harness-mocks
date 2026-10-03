package runner

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/sloprail/harness-mocks/internal/session"
)

// transcriptHasPreamble reports whether the transcript's FIRST non-empty record is a
// preamble type — the signature that seedPreamble has already run against this file.
// Only the first record is inspected: the preamble is a contiguous head block, so a
// preamble type anywhere else would not be one this wrote, and checking the head is
// both sufficient and cheap.
func transcriptHasPreamble(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &rec) != nil {
			return false
		}
		return preambleTypes[rec.Type]
	}
	return false
}

// scanTranscriptTail reads the transcript at path and returns two facts the resume
// append needs: the uuid of the LAST record that carries one (the continuation's parent,
// "" when none does), and the string content of the LAST user record (to detect a
// re-entry that already wrote this prompt). Best-effort: a missing/unreadable file yields
// two empty strings, and the caller degrades to a derived parent and no dedup.
//
// Only user records with a plain STRING content are considered for lastUserContent — a
// human turn's content is a bare string, whereas a tool_result-carrying user record's
// content is a block list; comparing the latter to the prompt would never match and
// must not, so it is simply skipped here.
func scanTranscriptTail(path string) (lastUUID, lastUserContent string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			Type    string `json:"type"`
			UUID    string `json:"uuid"`
			Message *struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec.UUID != "" {
			lastUUID = rec.UUID
		}
		if rec.Type == "user" && rec.Message != nil {
			var s string
			if json.Unmarshal(rec.Message.Content, &s) == nil {
				lastUserContent = s
			}
		}
	}
	return lastUUID, lastUserContent
}

// claudeForkSchema is how Claude Code's records show a fork what to carry: the
// compaction's boundary, its summary, and the records the boundary preserved.
var claudeForkSchema = session.ForkSchema{
	UUID: chainUUID, Parent: chainParent, SessionKey: "sessionId",
	IsBoundary: isCompactBoundary, IsSummary: isCompactSummary, Preserved: preservedUUIDs,
}

// forkSegment is the records of a transcript a fork carries, under session id
// newID (see forkTranscript).
func forkSegment(data []byte, newID string) []map[string]any {
	return session.Fork(session.ParseRecords(data, chainUUID), newID, claudeForkSchema)
}

// ResumeTarget is the session id a `--resume` value names: the path of a session's
// transcript file (claude 2.1.285 resumed one by its path and appended to that file;
// recorded: snapshots/runs/resume-path), the id of a session (of this directory's project or another's), or the name a session was given (recorded: resume-name). A value that is
// none of them is returned as it is, for the run to fail on as an unknown session.
//
// sr:provides session-resume/claude
func ResumeTarget(configDir, cwd, value string) string {
	if id, ok := session.IDOfPath(claudeLayout, value); ok {
		return id
	}
	if session.Find(claudeLayout, resolveConfigDir(configDir), cwd, value) != "" {
		return value
	}
	named := func(path string) bool {
		data, err := os.ReadFile(path)
		return err == nil && transcriptAgentName(data) == value
	}
	if id := session.Latest(claudeLayout, resolveConfigDir(configDir), cwd, named); id != "" {
		return id
	}
	return value
}

// LatestSession is the id `--continue` resumes: the directory's most recently
// written session, or "" (recorded: snapshots/runs/resume-continue).
//
// sr:provides session-resume/claude
func LatestSession(configDir, cwd string) string {
	return session.Latest(claudeLayout, resolveConfigDir(configDir), cwd, func(string) bool { return true })
}

// transcriptAgentName is the last agent-name a transcript records, a session's name.
func transcriptAgentName(data []byte) string {
	name := ""
	for _, line := range strings.Split(string(data), "\n") {
		var rec struct{ Type, AgentName string }
		if strings.Contains(line, `"agent-name"`) && json.Unmarshal([]byte(line), &rec) == nil && rec.Type == "agent-name" {
			name = rec.AgentName
		}
	}
	return name
}

// ForgetSession removes what a session left on disk, its transcript and its
// sub-agents' beside it: `--no-session-persistence` leaves nothing to resume
// (recorded: snapshots/runs/no-session-persistence wrote no transcript at all).
// The mock needs the file while it runs, so it is removed when the run is over.
//
// sr:provides session-resume/claude
func ForgetSession(configDir, cwd, id string) {
	path := sessionFilePath(resolveConfigDir(configDir), cwd, id)
	_ = os.Remove(path)
	_ = os.RemoveAll(strings.TrimSuffix(path, claudeLayout.Ext))
}
