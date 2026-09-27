package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/claude-mock/internal/hooks"
)

// transcript is one run's handle on the session record it writes — the file,
// the chain through it, and the path the harness REPORTS for it, which are not
// always the same thing.
//
// # Why the file is opened lazily
//
// Real Claude Code does not write a fresh session's transcript before
// SessionStart. Measured across every real transcript on one machine: all 940
// `SessionStart:startup` runs of a hook that read its own transcript found no
// file, and in every one the transcript's origin record (its first parentless
// record) was the attachment recording that very SessionStart hook's result —
// written after the hook exited. When no SessionStart hook prints anything, no
// attachment is written and the origin is the user's prompt. So the mock does
// not create the file until the first record is written, and the first record
// of a fresh session is whatever the harness writes first.
//
// # Reported path vs actual path
//
// A session resumed from a different working directory keeps appending to the
// transcript where it began, but real Claude Code reports transcript_path under
// the project directory of the directory it was resumed in — a file that does
// not exist. Measured: a session begun in a worktree and resumed from the main
// checkout got a SessionStart:resume payload naming
// <projects>/<main checkout>/<id>.jsonl, while that hook's own attachment and
// every later record went to <projects>/<worktree>/<id>.jsonl. The mock models
// it the same way: `path` is where records go, `reported` is what payloads say.
type transcript struct {
	path     string // where records are written
	reported string // what hook payloads carry as transcript_path
	f        *os.File
	sw       *sessionWriter
	stamp    recordStamp

	// fresh marks a transcript this run is opening for the first time, so the
	// preamble real Claude Code opens a file with is written ahead of the first
	// record.
	fresh bool
}

// recordStamp is what the harness writes on every record it persists, filled
// in only where a record does not already carry the field.
type recordStamp struct {
	SessionID   string
	Cwd         string
	IsSidechain bool
	AgentID     string
}

// openTranscript returns a handle on path. The file is opened now when it
// already exists (a resume, a nested sub-agent run, a caller that pre-seeded it)
// and on the first write otherwise.
func openTranscript(path, reported string, stamp recordStamp) (*transcript, error) {
	t := &transcript{path: path, reported: reported, stamp: stamp}
	if t.reported == "" {
		t.reported = path
	}
	if _, err := os.Stat(path); err == nil {
		if err := t.open(); err != nil {
			return nil, err
		}
	} else {
		t.fresh = true
	}
	return t, nil
}

func (t *transcript) open() error {
	if t.f != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	t.f = f
	t.sw = newSessionWriter(f)
	t.sw.stamp = t.stamp
	return nil
}

// ensure opens the file for writing, writing the preamble first when this run
// is the one creating it.
func (t *transcript) ensure() {
	if t == nil || t.f != nil {
		return
	}
	if err := t.open(); err != nil {
		return
	}
	if t.fresh && !t.stamp.IsSidechain {
		for _, line := range mockPreambleRecords(t.stamp.SessionID) {
			appendToSession(t.f, line)
		}
	}
}

// file is the open file, opening it if needed — for code that still speaks in
// *os.File (the script's A10N_MOCK_SESSION_FILE, the legacy seeders).
func (t *transcript) file() *os.File {
	t.ensure()
	return t.f
}

// Close closes the file if it was ever opened.
func (t *transcript) Close() {
	if t != nil && t.f != nil {
		t.f.Close()
	}
}

// exists reports whether the file has been written.
func (t *transcript) exists() bool {
	_, err := os.Stat(t.path)
	return err == nil
}

// persist writes one record, chained after the one before it.
func (t *transcript) persist(line []byte) {
	if t == nil {
		return
	}
	t.ensure()
	t.sw.persist(line)
}

// persistMap marshals rec and persists it.
func (t *transcript) persistMap(rec map[string]any) {
	if b, err := marshalRecord(rec); err == nil {
		t.persist(b)
	}
}

// lastUUID is the uuid of the last record written (or already on disk).
func (t *transcript) lastUUID() string {
	if t == nil || t.sw == nil {
		if t != nil && t.exists() {
			return lastRecordUUID(t.path)
		}
		return ""
	}
	return t.sw.lastUUID
}

// recordHookRuns writes the attachment records real Claude Code writes for one
// fired hook event — one per handler that did something worth recording.
//
// The shapes, and when each is written, are taken from every hook attachment
// in the real transcripts on one machine (~8,000 of them):
//
//   - hook_success {command, content, stdout, stderr, exitCode, durationMs}:
//     a handler that exited 0 and printed anything. A handler that printed
//     nothing leaves no record at all — 0 of 2,846 PreToolUse successes are
//     empty. content is stdout when it is plain text (SessionStart context),
//     empty when stdout is a JSON control object.
//   - hook_additional_context {content: [text]}: a JSON additionalContext.
//   - hook_non_blocking_error {stderr: "Failed with non-blocking status code:
//     …", stdout, exitCode, command, durationMs}: any exit other than 0 or 2.
//   - hook_blocking_error {blockingError: {blockingError: reason}}: exit 2, or
//     a Stop/SubagentStop decision:block.
//
// Every one carries hookName ("PreToolUse:Bash", "SessionStart:startup",
// "Stop"), hookEvent, and toolUseID — the tool call's id for a tool event, a
// fresh uuid otherwise. They are chained like any record, and where they land
// follows from when the hook fires: a PreToolUse attachment after its tool_use,
// a PostToolUse one after the result, a fresh session's SessionStart one first
// of all — which makes it the transcript's origin.
func (t *transcript) recordHookRuns(in hooks.Input, runs []hooks.HandlerRun) {
	if t == nil {
		return
	}
	hookName := string(in.HookEventName)
	switch in.HookEventName {
	case hooks.EventPreToolUse, hooks.EventPostToolUse:
		if in.ToolName != "" {
			hookName += ":" + in.ToolName
		}
	case hooks.EventSessionStart:
		if in.Source != "" {
			hookName += ":" + in.Source
		}
	}
	toolUseID := in.ToolUseID
	if toolUseID == "" {
		toolUseID = newRecordUUID()
	}
	stopLike := in.HookEventName == hooks.EventStop || in.HookEventName == hooks.EventSubagentStop
	for _, r := range runs {
		att := map[string]any{"hookName": hookName, "toolUseID": toolUseID, "hookEvent": string(in.HookEventName)}
		ac := additionalContextFrom(r.Output)
		switch {
		case r.Blocked:
			reason := strings.TrimSpace(r.Stderr)
			if stopLike {
				t.stopHookFeedback(reason)
			}
			att["type"] = "hook_blocking_error"
			att["blockingError"] = map[string]any{"blockingError": reason}
		case stopLike && r.Output.Decision == "block":
			t.stopHookFeedback(r.Output.Reason)
			att["type"] = "hook_blocking_error"
			att["blockingError"] = map[string]any{"blockingError": r.Output.Reason}
		case r.ExitCode != 0:
			msg := strings.TrimSpace(r.Stderr)
			if msg == "" {
				msg = "No stderr output"
			}
			att["type"] = "hook_non_blocking_error"
			att["stderr"] = "Failed with non-blocking status code: " + msg
			att["stdout"] = r.Stdout
			att["exitCode"] = r.ExitCode
			att["command"] = r.Command
			att["durationMs"] = r.DurationMs
		case ac != "":
			att["type"] = "hook_additional_context"
			att["content"] = []string{ac}
		case r.Stdout != "" || r.Stderr != "":
			content := ""
			if !looksLikeJSONObject(r.Stdout) {
				content = strings.TrimRight(r.Stdout, "\n")
			}
			att["type"] = "hook_success"
			att["content"] = content
			att["stdout"] = r.Stdout
			att["stderr"] = r.Stderr
			att["exitCode"] = r.ExitCode
			att["command"] = r.Command
			att["durationMs"] = r.DurationMs
		default:
			continue
		}
		t.persistMap(map[string]any{"type": "attachment", "attachment": att})
	}
}

func looksLikeJSONObject(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return false
	}
	var v map[string]any
	return json.Unmarshal([]byte(s), &v) == nil
}

// stopHookFeedback writes the record real Claude Code puts ahead of a blocking
// Stop's attachment: a meta user turn carrying the reason, which is what the
// re-prompted agent reads. Measured: in a real transcript the
// "Stop hook feedback:\n<reason>" user record sits immediately before the
// hook_blocking_error attachment, in the main file for Stop and in the
// sub-agent's own file for SubagentStop.
func (t *transcript) stopHookFeedback(reason string) {
	if t == nil || reason == "" {
		return
	}
	t.persistMap(map[string]any{
		"type":    "user",
		"isMeta":  true,
		"message": map[string]any{"role": "user", "content": "Stop hook feedback:\n" + reason},
	})
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
