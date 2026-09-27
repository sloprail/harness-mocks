package runner

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
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

// lastUUIDs is the uuids of the last n records written that carry one, oldest
// first.
func (t *transcript) lastUUIDs(n int) []string {
	if t == nil || n <= 0 {
		return []string{}
	}
	data, err := os.ReadFile(t.path)
	if err != nil {
		return []string{}
	}
	var all []string
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var rec struct {
			UUID string `json:"uuid"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.UUID != "" {
			all = append(all, rec.UUID)
		}
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	if all == nil {
		return []string{}
	}
	return all
}

// recordHookRuns writes what real Claude Code records for one fired hook
// event, handler by handler. Each rule below is pinned to evidence in
// claude-mock/EVIDENCE.md (real transcripts, controlled runs of claude
// 2.1.282, and the 2.1.282 binary's hook runner):
//
//   - exit 0, nothing printed: no record at all.
//   - exit 0 with output: hook_success {content, stdout, stderr, exitCode,
//     command, durationMs}; content is stdout when it is plain text, "" when
//     stdout is a JSON object. A JSON additionalContext then adds a
//     hook_additional_context {content: [text]} right after it.
//   - exit 0 with a JSON block — Stop/SubagentStop decision:block, PostToolUse
//     decision:block: the "Stop hook feedback" meta turn (Stop events) and a
//     hook_blocking_error {blockingError: {blockingError: reason, command}}.
//     A PreToolUse deny records nothing: the refusal is the tool_result.
//   - exit 2: SessionStart and SubagentStart write it as a
//     hook_non_blocking_error whose stderr is "[<command>]: <stderr>" (no
//     durationMs); Stop/SubagentStop write only the feedback meta turn
//     "Stop hook feedback:\n[<command>]: <stderr>"; PostToolUse writes a
//     hook_blocking_error; PreToolUse writes nothing (its tool_result carries
//     it). Other events: no evidence, nothing written.
//   - any other non-zero exit: hook_non_blocking_error {stderr: "Failed with
//     non-blocking status code: <stderr or No stderr output>", stdout,
//     exitCode, command, durationMs}.
//
// Every attachment carries hookName ("PreToolUse:Bash",
// "SessionStart:startup", "SubagentStart:<agent_type>", "Stop"), hookEvent and
// toolUseID — the tool call's id for a tool event, else one fresh uuid for the
// whole fire. A Stop fire that ran any handler ends with a stop_hook_summary
// record carrying the same toolUseID.
func (t *transcript) recordHookRuns(in hooks.Input, runs []hooks.HandlerRun) {
	if t == nil || !recordedEvents[in.HookEventName] {
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
	case hooks.EventSubagentStart:
		if in.AgentType != "" {
			hookName += ":" + in.AgentType
		}
	}
	toolUseID := in.ToolUseID
	if toolUseID == "" {
		toolUseID = newRecordUUID()
	}
	ev := in.HookEventName
	stopLike := ev == hooks.EventStop || ev == hooks.EventSubagentStop
	summary := stopSummary{toolUseID: toolUseID}
	att := func(typ string, fields map[string]any) {
		a := map[string]any{"type": typ, "hookName": hookName, "toolUseID": toolUseID, "hookEvent": string(ev)}
		for k, v := range fields {
			a[k] = v
		}
		t.persistMap(map[string]any{"type": "attachment", "attachment": a})
	}
	for _, r := range runs {
		info := map[string]any{"command": r.Command}
		ac := additionalContextFrom(r.Output)
		switch {
		case r.Blocked:
			quoted := hooks.QuoteBlock(r.Command, r.Stderr)
			switch ev {
			case hooks.EventSessionStart, hooks.EventSubagentStart:
				att("hook_non_blocking_error", map[string]any{
					"stderr": quoted, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command,
				})
			case hooks.EventStop, hooks.EventSubagentStop:
				t.stopHookFeedback(quoted)
				summary.errors = append(summary.errors, quoted)
			case hooks.EventPostToolUse:
				att("hook_blocking_error", map[string]any{
					"blockingError": map[string]any{"blockingError": quoted, "command": r.Command},
				})
			}
			summary.hasOutput = true
		case (stopLike || ev == hooks.EventPostToolUse) && r.Output.Decision == "block":
			reason := r.Output.Reason
			if reason == "" {
				reason = "Blocked by hook"
			}
			if stopLike {
				t.stopHookFeedback(reason)
			}
			att("hook_blocking_error", map[string]any{
				"blockingError": map[string]any{"blockingError": reason, "command": r.Command},
			})
			summary.errors = append(summary.errors, reason)
			summary.hasOutput = true
		case ev == hooks.EventPreToolUse && isDeny(r.Output):
			// The refusal is the tool_result (see scanLines); nothing else.
			info["durationMs"] = r.DurationMs
		case r.ExitCode != 0:
			msg := strings.TrimSpace(r.Stderr)
			if msg == "" {
				msg = "No stderr output"
			}
			msg = "Failed with non-blocking status code: " + msg
			att("hook_non_blocking_error", map[string]any{
				"stderr": msg, "stdout": r.Stdout, "exitCode": r.ExitCode, "command": r.Command, "durationMs": r.DurationMs,
			})
			info["durationMs"] = r.DurationMs
			summary.errors = append(summary.errors, msg)
			summary.hasOutput = true
		case r.Stdout != "" || r.Stderr != "":
			content := ""
			if !looksLikeJSONObject(r.Stdout) {
				content = strings.TrimRight(r.Stdout, "\n")
			}
			att("hook_success", map[string]any{
				"content": content, "stdout": r.Stdout, "stderr": r.Stderr, "exitCode": r.ExitCode,
				"command": r.Command, "durationMs": r.DurationMs,
			})
			if ac != "" {
				t.additionalContext(in, hookName, toolUseID, ac)
				summary.contexts = append(summary.contexts, ac)
			}
			info["durationMs"] = r.DurationMs
			summary.hasOutput = true
		default:
			info["durationMs"] = r.DurationMs
		}
		if r.Output.Continue != nil && !*r.Output.Continue {
			summary.prevented = true
			summary.stopReason = r.Output.StopReason
			if summary.stopReason == "" {
				summary.stopReason = "Stop hook prevented continuation"
			}
		}
		summary.infos = append(summary.infos, info)
	}
	if ev == hooks.EventStop {
		t.writeStopSummary(summary)
	}
}

// recordedEvents are the events whose hooks leave records in a transcript —
// the ones there is evidence for (real transcripts and controlled claude
// 2.1.282 runs; EVIDENCE.md). SessionEnd's output left nothing in a controlled
// run, PreCompact/PostCompact's is display text only (the 2.1.282 binary), and
// there is no evidence at all for WorktreeCreate/WorktreeRemove: no record.
var recordedEvents = map[hooks.EventName]bool{
	hooks.EventSessionStart:     true,
	hooks.EventUserPromptSubmit: true,
	hooks.EventPreToolUse:       true,
	hooks.EventPostToolUse:      true,
	hooks.EventStop:             true,
	hooks.EventSubagentStart:    true,
	hooks.EventSubagentStop:     true,
}

// additionalContext writes the hook_additional_context record that follows a
// hook's hook_success when its JSON carried additionalContext. SessionStart's
// is named after the event alone, with the event as its toolUseID — the shape
// claude 2.1.282 wrote in a controlled run and in 9 real transcripts; every
// other event's shares the hook_success's name and id.
func (t *transcript) additionalContext(in hooks.Input, hookName, toolUseID, ac string) {
	if in.HookEventName == hooks.EventSessionStart {
		hookName, toolUseID = string(hooks.EventSessionStart), string(hooks.EventSessionStart)
	}
	t.persistMap(map[string]any{"type": "attachment", "attachment": map[string]any{
		"type": "hook_additional_context", "content": []string{ac},
		"hookName": hookName, "toolUseID": toolUseID, "hookEvent": string(in.HookEventName),
	}})
}

// isDeny reports a PreToolUse JSON refusal: permissionDecision deny, or the
// deprecated top-level decision:block.
func isDeny(out hooks.Output) bool {
	return out.Decision == "block" ||
		(out.HookSpecificOutput != nil && out.HookSpecificOutput.PermissionDecision == "deny")
}

// denyReason is the text a PreToolUse JSON refusal is quoted with:
// permissionDecisionReason, else reason, else "Blocked by hook" (claude
// 2.1.282's hook-output parser).
func denyReason(out hooks.Output) string {
	if out.HookSpecificOutput != nil && out.HookSpecificOutput.PermissionDecisionReason != "" {
		return out.HookSpecificOutput.PermissionDecisionReason
	}
	if out.Reason != "" {
		return out.Reason
	}
	return "Blocked by hook"
}

// stopSummary accumulates one Stop fire's stop_hook_summary.
type stopSummary struct {
	toolUseID  string
	infos      []map[string]any
	errors     []string
	contexts   []string
	hasOutput  bool
	prevented  bool
	stopReason string
}

// writeStopSummary writes the system record real Claude Code writes after
// every Stop fire that ran at least one hook (8,272 in the real transcripts,
// all in main files; none for SubagentStop). A handler that blocked is listed
// without durationMs, as claude 2.1.282 lists it.
func (t *transcript) writeStopSummary(s stopSummary) {
	if len(s.infos) == 0 {
		return
	}
	errs := s.errors
	if errs == nil {
		errs = []string{}
	}
	ctxs := s.contexts
	if ctxs == nil {
		ctxs = []string{}
	}
	t.persistMap(map[string]any{
		"type": "system", "subtype": "stop_hook_summary",
		"hookCount": len(s.infos), "hookInfos": s.infos, "hookErrors": errs,
		"hookAdditionalContext": ctxs, "preventedContinuation": s.prevented,
		"stopReason": s.stopReason, "hasOutput": s.hasOutput, "level": "suggestion",
		"toolUseID": s.toolUseID,
	})
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
// re-prompted agent reads. In the real transcripts the
// "Stop hook feedback:\n<reason>" user record sits immediately before the
// hook_blocking_error attachment (1,417 Stop, 472 SubagentStop), in the main
// file for Stop and in the sub-agent's own file for SubagentStop.
func (t *transcript) stopHookFeedback(reason string) {
	if t == nil {
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
