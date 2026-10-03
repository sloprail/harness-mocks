package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/session"
)

// forkTranscript writes dest as a FORK of the session fromID under the new
// session id newID — what `--resume <id> --fork-session` leaves.
//
// Measured on claude (the run under claude-mock/snapshots/runs/forkresume) and on
// the real forks on one machine:
//
//   - A session never compacted forks whole: every record, origin included,
//     its parentUuid unchanged, with sessionId rewritten to the fork's — as
//     every 2.1.280+ fork on the machine did.
//   - A compacted session forks from its LAST compact_boundary: a verbatim copy
//     of the boundary (same uuid and logicalParentUuid; sessionId rewritten),
//     then what follows it up to and including the summary, then the records
//     the boundary lists as preserved — re-parented into one chain after the
//     summary — then everything after the summary, the first of it re-parented
//     onto the last preserved record. That is the order of all 19 real
//     transcripts that open on a compact_boundary.
//
// The source file is only read.
//
// sr:provides session-fork/claude
func forkTranscript(configDir, cwd, fromID, dest, newID string) error {
	if session.Exists(dest) {
		return fmt.Errorf("fork: %s already exists", dest)
	}
	src := sessionFilePathIfExists(configDir, cwd, fromID)
	if src == "" {
		return &ErrNoConversation{SessionID: fromID}
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("fork: %w", err)
	}
	segment := forkSegment(data, newID)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var buf []byte
	for _, line := range mockPreambleRecords(newID) {
		buf = append(append(buf, line...), '\n')
	}
	for _, rec := range segment {
		b, err := marshalRecord(rec)
		if err != nil {
			continue
		}
		buf = append(append(buf, b...), '\n')
	}
	return os.WriteFile(dest, buf, 0o644)
}

// ResumeTarget is the session id a `--resume` value names: the path of a session's
// transcript file (claude 2.1.285 resumed one by its path and appended to that file;
// recorded: snapshots/runs/resume-path), the id of a session, or the name a session
// was given (recorded: resume-name).
//
// sr:provides session-resume/claude
func ResumeTarget(configDir, cwd, value string) string {
	return session.Target(claudeLayout, resolveConfigDir(configDir), cwd, value, func(path string) bool { return sessionTitleOf(path) == value })
}

// LatestSession is the id `--continue` resumes: the most recently written session of
// the directory, or "" (recorded: snapshots/runs/resume-continue).
//
// sr:provides session-resume/claude
func LatestSession(configDir, cwd string) string {
	return session.Latest(claudeLayout, resolveConfigDir(configDir), cwd, func(string) bool { return true })
}

// sessionTitleOf is the name the transcript at path records for its session (its
// last agent-name record), or "".
func sessionTitleOf(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	name := ""
	for _, line := range strings.Split(string(data), "\n") {
		var rec struct{ Type, AgentName string }
		if strings.Contains(line, `"agent-name"`) && json.Unmarshal([]byte(line), &rec) == nil && rec.Type == "agent-name" {
			name = rec.AgentName
		}
	}
	return name
}
