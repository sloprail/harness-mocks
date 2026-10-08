package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/background-agent (captured with the real cursor-agent
// 2026.09.28): the agent dispatches a sub-agent with its Task tool, asking for
// the background, and ends its turn. Here the agent goes on with a command of
// its own, which outlasts the sub-agent's.
const bgTaskScript = `#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
n=${n:-0}
emit() { printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_%s","name":"%s","input":%s}]}}\n' "$n" "$1" "$2"; }
end() { printf '{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]}}\n{"type":"result","subtype":"success","result":"%s"}\n' "$1" "$1"; }
case "$n" in
0) emit Task "$(jq -nc --arg d "$TASK_DESC" --arg p "$TASK_PROMPT" --arg s "$SUB_SCRIPT" '{description: $d, prompt: $p, subagent_type: "shell", run_in_background: true, script: $s}')" ;;
1) emit Bash "$(jq -nc --arg c "$PARENT_CMD" '{command: $c}')" ;;
*) end LAUNCHED ;;
esac
`

// bgSubScript plays the sub-agent: its one command, then its reply.
const bgSubScript = `#!/bin/sh
if grep -q '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"SUBREPLY"}]}}' '{"type":"result","subtype":"success","result":"SUBREPLY"}'
else
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_s","name":"Bash","input":%s}]}}\n' "$(jq -nc --arg c "$SUB_CMD" '{command: $c}')"
fi
`

func bgStr(v any) string { s, _ := v.(string); return s }

// bgTaskFrames names the Task call's frames of a stream.
func bgTaskFrames(frames []map[string]any) (out []string) {
	for _, f := range frames {
		if n := frameName(f); strings.Contains(n, "taskToolCall") {
			out = append(out, n)
		}
	}
	return
}

// A sub-agent dispatched in the background: the agent is given a receipt at
// once (the call completes, saying the sub-agent runs in the background and
// naming its session), the turn goes on, and the sub-agent runs concurrently
// with it on its own session: its command's hooks, under its own session id,
// fire while the agent's own command is still running. The hooks that fire are
// the recording's: the Task call's preToolUse and no postToolUse, no
// subagentStart or subagentStop, the session starting and ending once
// (runs/background-agent).
// sr:proves background-agent/cursor
func TestSubAgentDispatchedInTheBackgroundGivesReceiptAndRunsConcurrently(t *testing.T) {
	sample := newestSample(t, "background-agent")
	setup := filepath.Join(sample, "..", "..", "setup")
	var task map[string]any
	var subCmd string
	var recordedHooks []map[string]any
	for _, p := range readJSONL(t, filepath.Join(sample, "payloads.jsonl")) {
		in, _ := p["tool_input"].(map[string]any)
		switch {
		case p["hook_event_name"] == "preToolUse" && p["tool_name"] == "Task":
			task = in
		case p["hook_event_name"] == "preToolUse" && p["tool_name"] == "Shell":
			subCmd, _ = in["command"].(string)
		}
		if p["hook_event_name"] != "afterAgentThought" && p["hook_event_name"] != "afterAgentResponse" {
			recordedHooks = append(recordedHooks, p)
		}
	}
	require.NotNil(t, task)
	require.Equal(t, true, task["run_in_background"])
	require.NotEmpty(t, subCmd)
	subCmd = strings.Replace(subCmd, "sleep 5", "sleep 1", 1) // done well within the agent's own command
	parentCmd := "sleep 3; echo PARENTDONE"

	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	script, sub := filepath.Join(scratch, "scenario.sh"), filepath.Join(scratch, "sub.sh")
	require.NoError(t, os.WriteFile(script, []byte(bgTaskScript), 0o755))
	require.NoError(t, os.WriteFile(sub, []byte(bgSubScript), 0o755))
	logPath := filepath.Join(scratch, "payloads.jsonl")
	prompt := bgStr(task["prompt"])
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + logPath,
		"TASK_DESC=" + bgStr(task["description"]), "TASK_PROMPT=" + prompt, "SUB_SCRIPT=" + sub,
		"SUB_CMD=" + subCmd, "PARENT_CMD=" + parentCmd}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	frames := readJSONLText(t, string(out))
	log := readJSONL(t, logPath)

	// the hooks the recording shows fired, and only those; the agent's own
	// command, which the recording lacks, aside
	counts := func(entries []map[string]any) map[string]int {
		n := map[string]int{}
		for _, p := range entries {
			e := bgStr(p["hook_event_name"])
			if e == "postToolUse" && p["tool_name"] == "Task" {
				e = "postToolUse(Task)"
			}
			n[e]++
		}
		return n
	}
	var fired []map[string]any
	for _, p := range log {
		in, _ := p["tool_input"].(map[string]any)
		if p["hook_event_name"] != nil && p["command"] != parentCmd && in["command"] != parentCmd {
			fired = append(fired, p)
		}
	}
	wantHooks := counts(recordedHooks)
	require.Equal(t, 1, wantHooks["sessionStart"])
	require.Equal(t, 1, wantHooks["sessionEnd"])
	require.Zero(t, wantHooks["subagentStart"]+wantHooks["subagentStop"]+wantHooks["postToolUse(Task)"])
	assert.Equal(t, wantHooks, counts(fired))

	// the call completes with a background receipt, as recorded
	var started, receipt map[string]any
	for _, f := range frames {
		if tc, _ := f["tool_call"].(map[string]any); tc["taskToolCall"] != nil {
			body := tc["taskToolCall"].(map[string]any)
			if f["subtype"] == "started" {
				started = body
			} else {
				result, _ := body["result"].(map[string]any)
				receipt, _ = result["success"].(map[string]any)
			}
		}
	}
	require.NotNil(t, started)
	assert.Equal(t, prompt, started["args"].(map[string]any)["prompt"])
	require.NotNil(t, receipt, "the Task call completed with a success")
	assert.Equal(t, true, receipt["isBackground"])
	agent := bgStr(receipt["agentId"])
	require.NotEmpty(t, agent)
	want := bgTaskFrames(readJSONL(t, filepath.Join(sample, "stream.jsonl")))
	require.Len(t, want, 2)
	assert.Equal(t, want, bgTaskFrames(frames), "the Task call's frames, as recorded")

	// the Task call's preToolUse hook saw what the recording's did; the
	// sub-agent has a session of its own, and its tool's hooks are the
	// recording's: preToolUse naming the command, postToolUse carrying what it
	// printed and its exit code
	var main string
	var order []string // the shell commands' hooks, in order, by whose they are
	var subPre, subPost int
	for _, p := range log {
		if p["tool_name"] == "Task" {
			assert.Equal(t, "preToolUse", p["hook_event_name"])
			assert.Equal(t, task, p["tool_input"])
			main = bgStr(p["session_id"])
		}
		if p["session_id"] == agent && p["tool_name"] == "Shell" {
			in, _ := p["tool_input"].(map[string]any)
			assert.Equal(t, subCmd, in["command"])
			switch p["hook_event_name"] {
			case "preToolUse":
				subPre++
			case "postToolUse":
				subPost++
				assert.JSONEq(t, `{"output":"SUBDONE\n","exitCode":0}`, bgStr(p["tool_output"]))
			}
		}
		if e := bgStr(p["hook_event_name"]); e == "beforeShellExecution" || e == "afterShellExecution" {
			who := "main"
			if p["session_id"] == agent {
				who = "sub-agent"
			}
			order = append(order, who+" "+e+" "+bgStr(p["command"]))
		}
	}
	require.NotEmpty(t, main)
	assert.NotEqual(t, main, agent)
	assert.Equal(t, []int{1, 1}, []int{subPre, subPost})
	// the agent's command begun, the sub-agent's run to its end, then the agent's ended
	index := func(s string) int {
		for i, o := range order {
			if o == s {
				return i
			}
		}
		return -1
	}
	begun, subEnd, ended := index("main beforeShellExecution "+parentCmd), index("sub-agent afterShellExecution "+subCmd), index("main afterShellExecution "+parentCmd)
	require.NotEqual(t, -1, subEnd)
	assert.Less(t, begun, subEnd)
	assert.Less(t, subEnd, ended)

	// the sub-agent's end is reported, its task the receipt's agent, as recorded
	var kinds []string
	var note map[string]any
	for _, f := range frames {
		kinds = append(kinds, bgStr(f["type"])+"/"+bgStr(f["subtype"]))
		if f["subtype"] == "task_notification" {
			note = f
		}
	}
	require.NotNil(t, note)
	assert.Equal(t, agent, note["task_id"])
	assert.Equal(t, "success", note["status"])
	assert.Contains(t, kinds, "system/task_notification")

	// the stream's order: the receipt (the Task call's completed frame) comes
	// first, the agent's own command goes on after it, and the sub-agent's end
	// is reported later, before the result, as the recording's stream orders
	// the Task call, its end notice and the result
	orderOf := func(fs []map[string]any) (out []string) {
		for _, f := range fs {
			tc, _ := f["tool_call"].(map[string]any)
			switch {
			case tc["taskToolCall"] != nil:
				out = append(out, "task/"+bgStr(f["subtype"]))
			case f["subtype"] == "task_notification":
				out = append(out, "task_notification")
			case f["type"] == "result":
				out = append(out, "result")
			}
		}
		return
	}
	recordedOrder := orderOf(readJSONL(t, filepath.Join(sample, "stream.jsonl")))
	require.Equal(t, []string{"task/started", "task/completed", "task_notification", "result"}, recordedOrder)
	assert.Equal(t, recordedOrder, orderOf(frames))
	idx := func(pred func(map[string]any) bool) int {
		for i, f := range frames {
			if pred(f) {
				return i
			}
		}
		return -1
	}
	receiptAt := idx(func(f map[string]any) bool {
		tc, _ := f["tool_call"].(map[string]any)
		return tc["taskToolCall"] != nil && f["subtype"] == "completed"
	})
	parentShellAt := idx(func(f map[string]any) bool {
		tc, _ := f["tool_call"].(map[string]any)
		return tc["shellToolCall"] != nil && f["subtype"] == "started"
	})
	noteAt := idx(func(f map[string]any) bool { return f["subtype"] == "task_notification" })
	require.NotEqual(t, -1, receiptAt)
	require.NotEqual(t, -1, parentShellAt)
	// the receipt did not wait for the sub-agent: the agent's own command
	// began (above: before the sub-agent's end) after it, and the end notice
	// follows both
	assert.Less(t, receiptAt, parentShellAt, "the receipt comes before the agent's own command begins")
	assert.Less(t, parentShellAt, noteAt, "the sub-agent's end is reported after the agent's own command began")

	// the payload fields the recording shows: the sub-agent's command ran
	// outside a sandbox and printed SUBDONE, the session starts and ends as
	// the foreground agent's own, and is not a background agent
	recordedPayload := func(entries []map[string]any, event string) map[string]any {
		for _, p := range entries {
			if p["hook_event_name"] == event {
				return p
			}
		}
		return nil
	}
	recSubDone := recordedPayload(recordedHooks, "afterShellExecution")
	require.NotNil(t, recSubDone)
	var subDone map[string]any
	for _, p := range log {
		if p["hook_event_name"] == "afterShellExecution" && p["session_id"] == agent {
			subDone = p
		}
	}
	require.NotNil(t, subDone)
	assert.Equal(t, recSubDone["output"], subDone["output"])
	assert.Equal(t, recSubDone["sandbox"], subDone["sandbox"])
	for event, fields := range map[string][]string{
		"sessionStart": {"is_background_agent"},
		"sessionEnd":   {"is_background_agent", "reason", "final_status"},
	} {
		rec, got := recordedPayload(recordedHooks, event), recordedPayload(log, event)
		require.NotNil(t, rec, event)
		require.NotNil(t, got, event)
		for _, f := range fields {
			assert.Equal(t, rec[f], got[f], event+"."+f)
		}
	}
}
