package runner

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// recordStamp is what the harness writes on every record it persists, filled
// in only where a record does not already carry the field. Every real user,
// assistant and attachment record carries all of these (190,464 user/assistant
// and 85,334 attachment records in the main transcripts: isSidechain, userType,
// entrypoint, version, gitBranch, cwd, sessionId, timestamp).
type recordStamp struct {
	SessionID   string
	Cwd         string
	IsSidechain bool
	AgentID     string
	// GitBranch is the cwd's branch; empty (and so left off, as real Claude
	// Code leaves it off) outside a git repository.
	GitBranch string
}

// Real Claude Code's own bookkeeping values for a `claude -p` session: a
// print-mode run records entrypoint "sdk-cli" (3,210 real records, and every
// record of the controlled 2.1.282 runs in EVIDENCE.md), userType "external",
// and its version. The mock models 2.1.282, the version its evidence was taken
// from.
const (
	stampUserType   = "external"
	stampEntrypoint = "sdk-cli"
	stampVersion    = "2.1.282"
)

// newRecordStamp is the stamp for a run in cwd.
func newRecordStamp(sessionID, cwd string) recordStamp {
	return recordStamp{SessionID: sessionID, Cwd: cwd, GitBranch: gitBranch(cwd)}
}

// gitBranch is what real Claude Code records as gitBranch: the checked-out
// branch, "HEAD" when detached, "" (field omitted) outside a repository.
func gitBranch(cwd string) string {
	if out, err := exec.Command("git", "-C", cwd, "symbolic-ref", "--short", "-q", "HEAD").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	if err := exec.Command("git", "-C", cwd, "rev-parse", "--git-dir").Run(); err == nil {
		return "HEAD"
	}
	return ""
}

// nowStamp is the timestamp format real records carry.
func nowStamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// marshalRecord encodes a transcript or stream record the way real Claude Code
// does: WITHOUT Go's HTML escaping. encoding/json turns <, > and & into \u003c,
// \u003e and \u0026 by default, so a <task-notification> turn, or a hook's
// stderr carrying "a && b", would be written as bytes no real transcript
// contains — and a reader grepping for the real text would not find it.
func marshalRecord(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
