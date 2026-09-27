package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These pin the background-task lifecycle and the hook records to claude
// 2.1.282: controlled `claude -p` runs of it, its binary, and the real
// transcripts on one machine (claude-mock/EVIDENCE.md).

// messageText is a record's message.content when it is a string.
func messageText(r rec) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(r.Message, &m) != nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(m.Content, &s)
	return s
}

// toolResultOf is the tool_result block answering id in recs, and its record.
func toolResultOf(t *testing.T, recs []rec, id string) (map[string]any, rec) {
	t.Helper()
	for _, r := range recs {
		var m struct {
			Content []map[string]any `json:"content"`
		}
		if r.Type != "user" || json.Unmarshal(r.Message, &m) != nil {
			continue
		}
		for _, b := range m.Content {
			if b["type"] == "tool_result" && b["tool_use_id"] == id {
				return b, r
			}
		}
	}
	t.Fatalf("no tool_result for %s", id)
	return nil, rec{}
}

func indexWhere(recs []rec, from int, f func(rec) bool) int {
	for i := from; i < len(recs); i++ {
		if f(recs[i]) {
			return i
		}
	}
	return -1
}

func isStopSummary(r rec) bool { return r.Subtype == "stop_hook_summary" }

// TestT017_11_BackgroundBashFinishedMidTurn: a background Bash is answered at
// once with the real receipt; when it finishes while the agent is still
// working, the notification is handed over INSIDE the turn — a queued_command
// attachment (commandMode task-notification) after the next tool result — and
// UserPromptSubmit fires with it. Stop fires after, at the end of the turn.
func TestT017_11_BackgroundBashFinishedMidTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	tmp := filepath.Join(dir, "tmp")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"UserPromptSubmit": h, "Stop": h})
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"echo BG-OUTPUT-4411","description":"make output","run_in_background":true}`),
		toolUse("fg1", "Bash", `{"command":"sleep 1"}`),
	)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_TMPDIR=" + tmp}, "--script", sc, "--session-id", "bg-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-1"))
	block, receipt := toolResultOf(t, recs, "bg1turn-s-a")
	id, _ := receipt.ToolUseResult["backgroundTaskId"].(string)
	require.Regexp(t, `^b[a-z0-9]{8}$`, id)
	root, err := filepath.EvalSymlinks(filepath.Join(tmp, "claude-"+strconv.Itoa(os.Getuid())))
	require.NoError(t, err)
	outFile := filepath.Join(root, encode(t, dir), "bg-1", "tasks", id+".output")
	assert.Equal(t, "Command running in background with ID: "+id+". Output is being written to: "+outFile+
		". You will be notified when it completes. To check interim output, use Read on that file path.", block["content"])
	assert.NotContains(t, receipt.ToolUseResult, "backgroundCwdHint", "no cd, no cwd note")
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, "BG-OUTPUT-4411\n\n[exited with code 0]\n", string(data))

	_, fg := toolResultOf(t, recs, "fg1turn-s-b")
	fgAt := indexWhere(recs, 0, func(r rec) bool { return r.UUID == fg.UUID })
	q := recs[fgAt+1]
	require.Equal(t, "attachment", q.Type, "the notification is handed over right after the tool result it arrived during")
	assert.Equal(t, "queued_command", q.Attachment["type"])
	assert.Equal(t, "task-notification", q.Attachment["commandMode"])
	note := "<task-notification>\n<task-id>" + id + "</task-id>\n<tool-use-id>bg1turn-s-a</tool-use-id>\n<output-file>" + outFile +
		"</output-file>\n<status>completed</status>\n<summary>Background command \"make output\" completed (exit code 0)</summary>\n</task-notification>"
	assert.Equal(t, note, q.Attachment["prompt"])
	stopAt := indexWhere(recs, 0, isStopSummary)
	assert.Greater(t, stopAt, fgAt+1, "Stop fires after, at the end of the turn")
	assert.Equal(t, -1, indexWhere(recs, stopAt+1, isStopSummary), "and once: nothing was left to hand over")

	var prompts []any
	var stops []map[string]any
	for _, p := range payloads(t, log) {
		switch p["hook_event_name"] {
		case "UserPromptSubmit":
			prompts = append(prompts, p["prompt"])
		case "Stop":
			stops = append(stops, p)
		}
	}
	assert.Equal(t, []any{"hello", note}, prompts, "UserPromptSubmit fires for the handed-over notification")
	require.Len(t, stops, 1)
	assert.Equal(t, []any{}, stops[0]["background_tasks"])
}

// TestT017_11b_BackgroundBashFailure: a background Bash that exits non-zero is
// announced as failed: `Background command "<description>" failed with exit
// code N`.
func TestT017_11b_BackgroundBashFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"echo x; exit 3","description":"fail","run_in_background":true}`),
		toolUse("fg1", "Bash", `{"command":"sleep 1"}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bg-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-2"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Attachment["type"] == "queued_command" })
	require.GreaterOrEqual(t, at, 0)
	prompt := recs[at].Attachment["prompt"].(string)
	assert.Contains(t, prompt, "<status>failed</status>")
	assert.Contains(t, prompt, `<summary>Background command "fail" failed with exit code 3</summary>`)
}

// TestT017_11c_BackgroundBashStillRunningIsStopped: in a `claude -p` session,
// a background Bash still running when the turn ends is listed in Stop's
// background_tasks, then killed: its stopped notification goes to the output
// stream only, never the transcript, and nothing waits for it. A command that
// changes directory gets the cwd note (claude 2.1.282).
func TestT017_11c_BackgroundBashStillRunningIsStopped(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"Stop": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"cd / && sleep 30","description":"long","run_in_background":true}`),
	)
	started := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bg-3",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(started), 15*time.Second, "nothing waits for the command")

	recs := readRecs(t, transcriptPath(t, cfg, dir, "bg-3"))
	block, receipt := toolResultOf(t, recs, "bg1turn-s-a")
	id := receipt.ToolUseResult["backgroundTaskId"].(string)
	resolved, _ := filepath.EvalSymlinks(dir)
	hint := "Session cwd remains " + resolved + "; directory changes made by the backgrounded command do not apply to subsequent commands."
	assert.True(t, strings.HasSuffix(block["content"].(string), "on that file path.\n"+hint), block["content"])
	assert.Equal(t, hint, receipt.ToolUseResult["backgroundCwdHint"])

	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, []any{map[string]any{"id": id, "type": "shell", "status": "running", "description": "long", "command": "cd / && sleep 30"}},
		ps[0]["background_tasks"])
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "bg-3"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "task-notification", "no notification is written for the stopped command")
	var frame map[string]any
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, `"subtype":"task_notification"`) {
			require.NoError(t, json.Unmarshal([]byte(l), &frame))
		}
	}
	require.NotNil(t, frame, "the stream carries the stopped notification")
	assert.Equal(t, "stopped", frame["status"])
	assert.Equal(t, id, frame["task_id"])
	assert.Equal(t, "long", frame["summary"])
}

// TestT017_12_BackgroundAgent: a background Agent is answered at once with the
// real receipt — content as a text block, toolUseResult with
// canReadOutputFile and resolvedModel, output_file a symlink to the
// sub-agent's own JSONL — and runs concurrently. The turn that launched it
// ends; Stop fires with the running sub-agent in background_tasks; the
// session waits for it; its notification (summary `Agent "<d>" finished`,
// note, result, usage) starts a new turn, after the first stop_hook_summary,
// with UserPromptSubmit fired for it; and Stop fires again at that turn's end.
func TestT017_12_BackgroundAgent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"UserPromptSubmit": h, "Stop": h})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
sleep 1
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY-7702"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY-7702"}'
`, 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"bg agent","script":"`+sub+`","run_in_background":true}`),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bga-1",
		"--project-dir", dir, "--config-dir", cfg, "--model", "haiku", "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "bga-1")
	recs := readRecs(t, main)
	block, receipt := toolResultOf(t, recs, "ag1turn-s-a")
	agentID, _ := receipt.ToolUseResult["agentId"].(string)
	require.Regexp(t, `^a[0-9a-f]{16}$`, agentID)
	outFile := receipt.ToolUseResult["outputFile"].(string)
	assert.Equal(t, map[string]any{
		"isAsync": true, "status": "async_launched", "agentId": agentID, "description": "bg agent",
		"prompt": "go", "outputFile": outFile, "canReadOutputFile": true, "resolvedModel": "haiku",
	}, receipt.ToolUseResult)
	receiptText := "Async agent launched successfully. (This tool result is internal metadata — never quote or paste any part of it, including the agentId below, into a user-facing reply.)\n" +
		"agentId: " + agentID + " (internal ID - do not mention to user. Use SendMessage with to: '" + agentID + "', summary: '<5-10 word recap>' to continue this agent.)\n" +
		"The agent is working in the background. You will be notified automatically when it completes. You know nothing about its results until that notification arrives — do not report, assume, or predict them; continue other work or respond to the user in the meantime.\n" +
		"Do not duplicate this agent's work — avoid working with the same files or topics it is using.\n" +
		"output_file: " + outFile + "\n" +
		"Do NOT Read or tail this file via the shell tool — it is the full subagent JSONL transcript and reading it will overflow your context. If the user asks for progress, say the agent is still running; you'll get a completion notification."
	assert.Equal(t, []any{map[string]any{"type": "text", "text": receiptText}}, block["content"])
	target, err := os.Readlink(outFile)
	require.NoError(t, err, "output_file is a symlink")
	assert.Equal(t, filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-"+agentID+".jsonl"), target)
	side, err := os.ReadFile(outFile)
	require.NoError(t, err, "and it resolves to the sub-agent's transcript")
	assert.Contains(t, string(side), "AGENT-REPLY-7702")

	first := indexWhere(recs, 0, isStopSummary)
	require.GreaterOrEqual(t, first, 0)
	n := first + 1
	require.Less(t, n, len(recs))
	notification := recs[n]
	require.Equal(t, "user", notification.Type, "the notification starts the next turn, right after the first Stop")
	require.NotNil(t, notification.ParentUUID)
	assert.Equal(t, recs[first].UUID, *notification.ParentUUID)
	var full map[string]any
	require.NoError(t, json.Unmarshal([]byte(notification.Raw), &full))
	assert.Equal(t, map[string]any{"kind": "task-notification"}, full["origin"])
	assert.Equal(t, "system", full["promptSource"])
	assert.Equal(t, "task_notification", full["turnOrigin"])
	note := messageText(notification)
	assert.True(t, strings.HasPrefix(note, "<task-notification>\n<task-id>"+agentID+"</task-id>\n<tool-use-id>ag1turn-s-a</tool-use-id>\n<output-file>"+outFile+
		"</output-file>\n<status>completed</status>\n<summary>Agent \"bg agent\" finished</summary>\n<note>A task-notification fires each time this agent stops with no live background children of its own. The user can send it another message and resume it, so the same task-id may notify more than once.</note>\n<result>AGENT-REPLY-7702</result>\n<usage><subagent_tokens>0</subagent_tokens><tool_uses>0</tool_uses><duration_ms>"), note)
	assert.True(t, strings.HasSuffix(note, "</duration_ms></usage>\n</task-notification>"), note)
	second := indexWhere(recs, n+1, isStopSummary)
	assert.Greater(t, second, n, "Stop fires again at the end of the notification's turn")

	var stops []map[string]any
	var prompts []any
	for _, p := range payloads(t, log) {
		switch p["hook_event_name"] {
		case "Stop":
			stops = append(stops, p)
		case "UserPromptSubmit":
			prompts = append(prompts, p["prompt"])
		}
	}
	require.Len(t, stops, 2)
	assert.Equal(t, []any{map[string]any{"id": agentID, "type": "subagent", "status": "running", "description": "bg agent", "agent_type": "general-purpose"}},
		stops[0]["background_tasks"], "the first Stop sees the sub-agent still running")
	assert.Contains(t, stops[0]["last_assistant_message"], "LAUNCHED")
	assert.Equal(t, []any{}, stops[1]["background_tasks"])
	assert.Equal(t, []any{"hello", note}, prompts)
}

// TestT017_12b_BackgroundAgentFailure: a background Agent whose run fails is
// announced as failed — `Agent "<d>" failed: <error>` — with no usage.
func TestT017_12b_BackgroundAgentFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := write(t, filepath.Join(dir, "sub.sh"), "#!/bin/sh\necho not-json\n", 0o755)
	sc := script(t, dir, "s",
		toolUse("ag1", "Agent", `{"prompt":"go","description":"bad agent","script":"`+sub+`","run_in_background":true}`),
	)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bga-2",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	var note string
	for _, r := range readRecs(t, transcriptPath(t, cfg, dir, "bga-2")) {
		if s := messageText(r); strings.HasPrefix(s, "<task-notification>") {
			note = s
		}
	}
	require.NotEmpty(t, note)
	assert.Contains(t, note, "<status>failed</status>")
	assert.Contains(t, note, `<summary>Agent "bad agent" failed: `)
	assert.NotContains(t, note, "<usage>")
}

// TestT017_12c_AgentWithoutRequiredInputIsRefused: description and prompt are
// required by the real Agent input schema; a call without them is refused with
// an InputValidationError tool_result, and nothing runs.
func TestT017_12c_AgentWithoutRequiredInputIsRefused(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SubagentStart": payloadLogger(t, dir, "log.sh", log, "")})
	sc := script(t, dir, "s", toolUse("ag1", "Agent", `{"prompt":"go","run_in_background":true}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bga-3",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	block, r := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "bga-3")), "ag1turn-s-a")
	assert.Equal(t, "<tool_use_error>InputValidationError: Agent failed due to the following issue:\nThe required parameter `description` is missing</tool_use_error>", block["content"])
	assert.Equal(t, true, block["is_error"])
	assert.NotContains(t, r.Raw, "async_launched")
	_, err := os.Stat(log)
	assert.True(t, os.IsNotExist(err), "no sub-agent started")
}

// TestT017_13_NestedSubAgents: a sub-agent that dispatches its own sub-agent.
// Each gets its own file in the session's one subagents/ directory; each
// file's records carry that sub-agent's id; the inner sub-agent's tool call
// reports its own agent_id; the dispatcher's file holds no sidechain record.
func TestT017_13_NestedSubAgents(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SubagentStart": h, "PreToolUse": h})
	inner := script(t, dir, "inner", toolUse("in1", "Bash", `{"command":"echo INNER-RAN"}`))
	outer := script(t, dir, "outer", toolUse("out1", "Agent", `{"prompt":"inner work","description":"inner","script":"`+inner+`"}`))
	root := script(t, dir, "root", toolUse("r1", "Agent", `{"prompt":"outer work","description":"outer","script":"`+outer+`"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "nest-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "nest-1")
	files, err := filepath.Glob(filepath.Join(strings.TrimSuffix(main, ".jsonl"), "subagents", "agent-*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 2, "both sub-agents write their own file, side by side")
	var starts []string
	innerID := ""
	for _, p := range payloads(t, log) {
		if p["hook_event_name"] == "SubagentStart" {
			starts = append(starts, p["agent_id"].(string))
			assert.Equal(t, main, p["transcript_path"])
		}
		if p["hook_event_name"] == "PreToolUse" && strings.HasPrefix(p["tool_use_id"].(string), "in1") {
			innerID, _ = p["agent_id"].(string)
		}
	}
	require.Len(t, starts, 2)
	assert.NotEqual(t, starts[0], starts[1])
	assert.Equal(t, starts[1], innerID, "the inner sub-agent's tool call carries its own agent_id")
	for _, f := range files {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "agent-"), ".jsonl")
		for _, r := range readRecs(t, f) {
			assert.True(t, r.IsSidechain)
			assert.Equal(t, id, r.AgentID)
		}
	}
	raw, err := os.ReadFile(main)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"isSidechain":true`)
	assert.NotContains(t, string(raw), "INNER-RAN")
}

// TestT017_14_StopFeedbackInTheMainFile: a Stop block is recorded in the
// session's own file the way claude 2.1.282 records it. A JSON decision:block
// leaves the "Stop hook feedback:\n<reason>" meta turn, a hook_blocking_error
// {blockingError: {blockingError, command}}, then a stop_hook_summary (same
// toolUseID, hookErrors [reason], hasOutput true). An exit 2 leaves the
// feedback quoting "[<command>]: <stderr>" and the summary, no attachment.
// stop_hook_active is set on every Stop after a block.
func TestT017_14_StopFeedbackInTheMainFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	counter := filepath.Join(dir, "n")
	hook := payloadLogger(t, dir, "stop.sh", log, `N=$(cat `+counter+` 2>/dev/null || echo 0); N=$((N+1)); echo $N > `+counter+`
if [ $N = 1 ]; then echo '{"decision":"block","reason":"JSON-REASON"}'; exit 0; fi
if [ $N = 2 ]; then echo "EXIT2-REASON" 1>&2; exit 2; fi`)
	settings(t, dir, map[string]string{"Stop": hook})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"turn text"}]}}'
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "stop-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	recs := readRecs(t, transcriptPath(t, cfg, dir, "stop-1"))
	fb := indexWhere(recs, 0, func(r rec) bool { return strings.HasPrefix(messageText(r), "Stop hook feedback:") })
	require.GreaterOrEqual(t, fb, 0)
	assert.Equal(t, "Stop hook feedback:\nJSON-REASON", messageText(recs[fb]))
	assert.Contains(t, recs[fb].Raw, `"isMeta":true`)
	att := recs[fb+1]
	assert.Equal(t, "hook_blocking_error", att.Attachment["type"])
	assert.Equal(t, map[string]any{"blockingError": "JSON-REASON", "command": hook}, att.Attachment["blockingError"])
	sum := recs[fb+2]
	require.True(t, isStopSummary(sum))
	var s1 map[string]any
	require.NoError(t, json.Unmarshal([]byte(sum.Raw), &s1))
	assert.Equal(t, att.Attachment["toolUseID"], s1["toolUseID"])
	assert.Equal(t, []any{"JSON-REASON"}, s1["hookErrors"])
	assert.Equal(t, true, s1["hasOutput"])
	assert.EqualValues(t, 1, s1["hookCount"])
	assert.Equal(t, []any{map[string]any{"command": hook}}, s1["hookInfos"], "a blocking hook is listed without durationMs")
	assert.Equal(t, "suggestion", s1["level"])

	fb2 := indexWhere(recs, fb+3, func(r rec) bool { return strings.HasPrefix(messageText(r), "Stop hook feedback:") })
	require.Greater(t, fb2, fb)
	assert.Equal(t, "Stop hook feedback:\n["+hook+"]: EXIT2-REASON\n", messageText(recs[fb2]))
	require.True(t, isStopSummary(recs[fb2+1]), "an exit-2 block leaves no attachment")
	last := indexWhere(recs, fb2+2, isStopSummary)
	require.Greater(t, last, fb2)
	var s3 map[string]any
	require.NoError(t, json.Unmarshal([]byte(recs[last].Raw), &s3))
	assert.Equal(t, false, s3["hasOutput"])
	assert.Equal(t, []any{}, s3["hookErrors"])

	var active []any
	for _, p := range payloads(t, log) {
		active = append(active, p["stop_hook_active"])
	}
	assert.Equal(t, []any{false, true, true}, active)
}

// TestT017_15_AdditionalContext: a hook whose JSON carries additionalContext
// leaves a hook_success (content "") and then a hook_additional_context — for
// PostToolUse under the tool call's name and id, for SessionStart named
// "SessionStart" with "SessionStart" as its toolUseID (claude 2.1.282).
func TestT017_15_AdditionalContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	post := write(t, filepath.Join(dir, "post.sh"), `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"POST-CTX"}}'
`, 0o755)
	ss := write(t, filepath.Join(dir, "ss.sh"), `#!/bin/sh
cat >/dev/null
echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"SS-CTX"}}'
`, 0o755)
	settings(t, dir, map[string]string{"PostToolUse": post, "SessionStart": ss})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "ac-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ac-1"))
	var kinds []string
	for i, r := range recs {
		if r.Type != "attachment" {
			continue
		}
		kinds = append(kinds, r.Attachment["type"].(string)+":"+r.Attachment["hookName"].(string))
		if r.Attachment["type"] == "hook_additional_context" {
			prev := recs[i-1]
			assert.Equal(t, "hook_success", prev.Attachment["type"], "it follows the hook's hook_success")
			assert.Equal(t, "", prev.Attachment["content"], "whose content is empty for a JSON stdout")
		}
	}
	assert.Equal(t, []string{
		"hook_success:SessionStart:startup", "hook_additional_context:SessionStart",
		"hook_success:PostToolUse:Bash", "hook_additional_context:PostToolUse:Bash",
	}, kinds)
	for _, r := range recs {
		if r.Attachment["type"] != "hook_additional_context" {
			continue
		}
		if r.Attachment["hookEvent"] == "SessionStart" {
			assert.Equal(t, "SessionStart", r.Attachment["toolUseID"])
			assert.Equal(t, []any{"SS-CTX"}, r.Attachment["content"])
		} else {
			assert.True(t, strings.HasPrefix(r.Attachment["toolUseID"].(string), "b1"))
			assert.Equal(t, []any{"POST-CTX"}, r.Attachment["content"])
		}
	}
}

// TestT017_16_NoStderrOutput: a Stop hook that fails without writing to stderr
// is recorded as "Failed with non-blocking status code: No stderr output", and
// its stop_hook_summary lists the error.
func TestT017_16_NoStderrOutput(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := write(t, filepath.Join(dir, "stop.sh"), "#!/bin/sh\ncat >/dev/null\nexit 1\n", 0o755)
	settings(t, dir, map[string]string{"Stop": hook})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "ne-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ne-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Attachment["type"] == "hook_non_blocking_error" })
	require.GreaterOrEqual(t, at, 0)
	a := recs[at].Attachment
	assert.Equal(t, "Stop", a["hookName"])
	assert.Equal(t, "Failed with non-blocking status code: No stderr output", a["stderr"])
	assert.EqualValues(t, 1, a["exitCode"])
	assert.Equal(t, hook, a["command"])
	assert.Contains(t, a, "durationMs")
	require.True(t, isStopSummary(recs[at+1]))
	var s map[string]any
	require.NoError(t, json.Unmarshal([]byte(recs[at+1].Raw), &s))
	assert.Equal(t, []any{"Failed with non-blocking status code: No stderr output"}, s["hookErrors"])
	assert.Equal(t, true, s["hasOutput"])
}

// TestT017_17_UnknownResume: --resume of a session no transcript holds fails
// the way claude 2.1.282 fails it: "No conversation found with session ID:
// <id>" on stderr, an error result frame on stdout, exit 1, no SessionStart,
// SessionEnd fired, nothing written.
func TestT017_17_UnknownResume(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "SessionEnd": h})
	for _, extra := range [][]string{nil, {"--fork-session", "--session-id", "new-fork"}} {
		args := append([]string{"--script", script(t, dir, "s"), "--resume", "nosuch", "--project-dir", dir, "--config-dir", cfg}, extra...)
		out, code := runInDir(t, dir, nil, append(args, "-p", "hello")...)
		assert.Equal(t, 1, code, out)
		assert.Contains(t, out, "No conversation found with session ID: nosuch\n")
		assert.Contains(t, out, `"subtype":"error_during_execution"`)
		assert.Contains(t, out, `"errors":["No conversation found with session ID: nosuch"]`)
	}
	_, err := os.Stat(filepath.Join(cfg, "projects"))
	assert.True(t, os.IsNotExist(err), "nothing is written")
	var events []any
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"])
	}
	assert.Equal(t, []any{"SessionEnd", "SessionEnd"}, events)
}

// TestT017_18_PrintMode: in raw --print mode Stop carries the output as
// last_assistant_message, and SessionEnd ends a `claude -p` session with
// reason "other" (claude 2.1.282).
func TestT017_18_PrintMode(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"Stop": h, "SessionEnd": h})
	sc := write(t, filepath.Join(dir, "s.sh"), "#!/bin/sh\necho 'RAW PRINT OUTPUT'\n", 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pm-1", "--print",
		"--project-dir", dir, "--config-dir", cfg, "hello")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "RAW PRINT OUTPUT")
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "Stop", ps[0]["hook_event_name"])
	assert.Equal(t, "RAW PRINT OUTPUT", ps[0]["last_assistant_message"])
	assert.Equal(t, false, ps[0]["stop_hook_active"])
	assert.Equal(t, []any{}, ps[0]["background_tasks"])
	assert.Equal(t, "SessionEnd", ps[1]["hook_event_name"])
	assert.Equal(t, "other", ps[1]["reason"])
	assert.NotContains(t, ps[1], "source")
}

// TestT017_19_PromptAndSessionEndAttachments: a UserPromptSubmit hook that
// prints plain text leaves a hook_success whose content is that text; a
// SessionEnd hook's output leaves nothing (claude 2.1.282).
func TestT017_19_PromptAndSessionEndAttachments(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	ups := write(t, filepath.Join(dir, "ups.sh"), "#!/bin/sh\ncat >/dev/null\necho UPS-PLAIN-OUT\n", 0o755)
	se := write(t, filepath.Join(dir, "se.sh"), "#!/bin/sh\ncat >/dev/null\necho SE-OUT\necho SE-ERR 1>&2\n", 0o755)
	settings(t, dir, map[string]string{"UserPromptSubmit": ups, "SessionEnd": se})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "up-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "up-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Type == "attachment" })
	require.GreaterOrEqual(t, at, 1)
	assert.Equal(t, "e2e-root-up-1", recs[at-1].UUID, "it follows the prompt")
	assert.Equal(t, "hook_success", recs[at].Attachment["type"])
	assert.Equal(t, "UserPromptSubmit", recs[at].Attachment["hookName"])
	assert.Equal(t, "UPS-PLAIN-OUT", recs[at].Attachment["content"])
	raw, _ := os.ReadFile(transcriptPath(t, cfg, dir, "up-1"))
	assert.NotContains(t, string(raw), "SE-OUT")
	assert.NotContains(t, string(raw), "SE-ERR")
}

// TestT017_20_PostToolUsePayload: PostToolUse sends tool_response — the
// tool's structured result, a Bash's {stdout, stderr, interrupted, isImage,
// noOutputExpected} — never tool_output (docs, PostToolUse input; a claude
// 2.1.282 payload).
func TestT017_20_PostToolUsePayload(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo POST-OUT"}`)), "--session-id", "pt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.NotContains(t, ps[0], "tool_output")
	assert.Equal(t, map[string]any{"stdout": "POST-OUT", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false}, ps[0]["tool_response"])
}

// TestT017_21_InlineToolResultPostToolUseIsKeyedByTheCall: a tool_result the
// scenario wrote itself fires PostToolUse with the call's tool_use_id, and the
// attachment is keyed by it — as every one of the 1,651 real PostToolUse
// attachments is.
func TestT017_21_InlineToolResultPostToolUseIsKeyedByTheCall(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	post := write(t, filepath.Join(dir, "post.sh"), "#!/bin/sh\ncat >/dev/null\necho post-said 1>&2\n", 0o755)
	settings(t, dir, map[string]string{"PostToolUse": post})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_inline1","name":"AskUserQuestion","content":"yes"}]}}'
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "il-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "il-1"))
	at := indexWhere(recs, 0, func(r rec) bool { return r.Type == "attachment" })
	require.GreaterOrEqual(t, at, 0)
	assert.Equal(t, "PostToolUse:AskUserQuestion", recs[at].Attachment["hookName"])
	assert.Equal(t, "toolu_inline1", recs[at].Attachment["toolUseID"])
}

// TestT017_22_ForkSessionWithoutResumeIsAPlainStart: --fork-session without
// --resume changes nothing — claude 2.1.282 started a plain session (source
// startup) under --session-id.
func TestT017_22_ForkSessionWithoutResumeIsAPlainStart(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionStart": payloadLogger(t, dir, "log.sh", log, "")})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--fork-session", "--session-id", "fs-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, "startup", ps[0]["source"])
	root, _ := firstRoot(readRecs(t, transcriptPath(t, cfg, dir, "fs-1")))
	assert.Equal(t, "e2e-root-fs-1", root.UUID)
}

// TestT017_23_MainRecordsCarryRealBookkeeping: every record the session writes
// carries what every real one does — isSidechain false, userType, entrypoint
// "sdk-cli" (a `claude -p` run), version, gitBranch in a git repository.
func TestT017_23_MainRecordsCarryRealBookkeeping(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	gitDir := filepath.Join(dir, "repo")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	require.NoError(t, exec.Command("git", "init", "-q", "-b", "feature-x", gitDir).Run())
	out, code := runInDir(t, gitDir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "bk-1",
		"--project-dir", gitDir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, r := range readRecs(t, transcriptPath(t, cfg, gitDir, "bk-1")) {
		if r.UUID == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
		assert.Equal(t, false, m["isSidechain"], r.Raw)
		assert.Equal(t, "external", m["userType"])
		assert.Equal(t, "sdk-cli", m["entrypoint"])
		assert.Equal(t, "2.1.282", m["version"])
		assert.Equal(t, "feature-x", m["gitBranch"])
		assert.Equal(t, "bk-1", m["sessionId"])
		assert.NotEmpty(t, m["timestamp"])
	}
}

// TestT017_24_EmptyToolResult: a tool that returns nothing is recorded as
// "(<Tool> completed with no output)" — claude 2.1.282 replaces empty result
// content with it; the real transcripts hold 3,479 such Bash results and no
// empty one. The structured result keeps the empty stdout.
func TestT017_24_EmptyToolResult(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "em-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	block, r := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "em-1")), "b1turn-s-a")
	assert.Equal(t, "(Bash completed with no output)", block["content"])
	assert.Equal(t, false, block["is_error"])
	assert.Equal(t, "", r.ToolUseResult["stdout"])
	assert.Contains(t, out, `"content":"(Bash completed with no output)"`, "the stream carries it too")
}
