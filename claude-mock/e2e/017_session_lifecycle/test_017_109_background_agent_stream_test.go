package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kindsOf are the type/subtype of each stream frame, with the ones the mock never sends
// (thinking_tokens, commands_changed, rate_limit_event) left out and a run of assistant frames
// as one.
func kindsOf(frames []map[string]any) (out []string) {
	for _, f := range frames {
		k, _ := f["type"].(string)
		if sub, _ := f["subtype"].(string); sub != "" {
			k += "/" + sub
		}
		if k == "system/thinking_tokens" || k == "system/commands_changed" || k == "rate_limit_event" {
			continue
		}
		if k == "assistant" && len(out) > 0 && out[len(out)-1] == "assistant" {
			continue
		}
		out = append(out, k)
	}
	return out
}

// afterNotification is the kinds from the last task_notification frame on.
func afterNotification(kinds []string) []string {
	for i := len(kinds) - 1; i >= 0; i-- {
		if kinds[i] == "system/task_notification" {
			return kinds[i:]
		}
	}
	return nil
}

// TestT017_109_ResultsOfABackgroundAgentRunComeLast: a turn that ends while a background agent
// still works streams no result then; the agent's notification starts a turn that opens with an
// init frame, and the results of both turns follow that turn, in order, with result_index 0 and
// 1 (recording bgagent: task_notification, init, assistant, result, result).
// sr:proves print-waits-for-background-agents/claude
// sr:proves task-stream-frames/claude
func TestT017_109_ResultsOfABackgroundAgentRunComeLast(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgagent/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := afterNotification(kindsOf(streamFrames(t, string(data))))
	require.Equal(t, []string{"system/task_notification", "system/init", "assistant", "result/success", "result/success"}, want, "recorded")

	dir := t.TempDir()
	// the agent finishes only after the first Stop has fired, so that turn ends with it still working
	h := payloadLogger(t, dir, "log.sh", filepath.Join(dir, "payloads.log"), `case "$IN" in *'"hook_event_name":"Stop"'*) touch `+dir+`/stopped;; esac`)
	settings(t, dir, map[string]string{"Stop": h})
	sub := write(t, filepath.Join(dir, "sub.sh"), `#!/bin/sh
while [ ! -f `+dir+`/stopped ]; do sleep 0.05; done
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"AGENT-REPLY"}]}}'
echo '{"type":"result","subtype":"success","result":"AGENT-REPLY"}'
`, 0o755)
	sc := write(t, filepath.Join(dir, "main.sh"), `#!/bin/sh
if ! grep -q 'turn-a' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
cat <<'JSONL'
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"ag1turn-a","name":"Agent","input":{"prompt":"go","description":"bg agent","script":"`+sub+`","run_in_background":true}}]}}
JSONL
exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"LAUNCHED"}]}}'
echo '{"type":"result","subtype":"success","result":"LAUNCHED"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgs-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	frames := streamFrames(t, out)
	assert.Equal(t, want, afterNotification(kindsOf(frames)))
	var indexes []any
	for _, f := range frames {
		if f["type"] == "result" {
			indexes = append(indexes, f["result_index"])
		}
	}
	assert.Equal(t, []any{float64(0), float64(1)}, indexes)
}

// tailAfterResult is the frames after the last result frame, as type/subtype with the
// task_updated patch's status and the notification's status where they have one.
func tailAfterResult(frames []map[string]any) (out []string) {
	last := -1
	for i, f := range frames {
		if f["type"] == "result" {
			last = i
		}
	}
	for _, f := range frames[last+1:] {
		k := f["type"].(string) + "/" + f["subtype"].(string)
		switch f["subtype"] {
		case "task_updated":
			k += ":" + f["patch"].(map[string]any)["status"].(string)
		case "task_notification":
			k += ":" + f["status"].(string)
		case "background_tasks_changed":
			k += ":" + strconv.Itoa(len(f["tasks"].([]any)))
		}
		out = append(out, k)
	}
	return out
}

// TestT017_110_ABackgroundShellKilledAtExitIsReportedAfterTheResult: the result frame comes
// first; then, for the command the run kills when it ends, the task list now empty, the task
// updated to killed and its notification, stopped, in that order (recording bgbash).
// sr:proves background-bash-reaped-at-exit/claude
// sr:proves task-stream-frames/claude
func TestT017_110_ABackgroundShellKilledAtExitIsReportedAfterTheResult(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgbash/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := tailAfterResult(streamFrames(t, string(data)))
	require.Equal(t, []string{"system/background_tasks_changed:0", "system/task_updated:killed", "system/task_notification:stopped"}, want, "recorded")

	dir := t.TempDir()
	sc := script(t, dir, "s", toolUse("bg1", "Bash", `{"command":"sleep 30","description":"long","run_in_background":true}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgb-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, tailAfterResult(streamFrames(t, out)))
}

// taskFrames are the task_* frames of a stream as subtype with the description a started or
// progress frame names and the status a notification carries.
func taskFrames(frames []map[string]any) (out []string) {
	for _, f := range frames {
		sub, _ := f["subtype"].(string)
		if !strings.HasPrefix(sub, "task_") {
			continue
		}
		switch sub {
		case "task_started", "task_progress":
			sub += ":" + f["description"].(string)
		case "task_notification":
			sub += ":" + f["status"].(string)
		}
		out = append(out, sub)
	}
	return out
}

// TestT017_111_NestedForegroundAgentsStreamTheirTaskFrames: an agent that starts another in
// the foreground streams its own task_started, a task_progress naming the call it makes, then
// the inner agent's task_started, task_updated and task_notification, and the outer's
// task_updated and task_notification last (recording meta, whose first step is this).
// sr:proves nested-subagents/claude
// sr:proves task-stream-frames/claude
func TestT017_111_NestedForegroundAgentsStreamTheirTaskFrames(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/meta/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := taskFrames(streamFrames(t, string(data)))[:7]
	require.Equal(t, []string{"task_started:outer", "task_progress:inner", "task_started:inner", "task_updated", "task_notification:completed",
		"task_updated", "task_notification:completed"}, want, "recorded")

	dir := t.TempDir()
	inner := script(t, dir, "inner")
	outer := script(t, dir, "outer", toolUse("in1", "Agent", `{"prompt":"p","description":"inner","script":"`+inner+`"}`))
	orch := script(t, dir, "orch", toolUse("ou1", "Agent", `{"prompt":"p","description":"outer","script":"`+outer+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "nf-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, taskFrames(streamFrames(t, out)))
}

// TestT017_112_AFailedBackgroundShellIsReportedFailed: a background command that exits non-zero
// is reported with task_updated status failed and a task_notification of status failed whose
// summary names it and its exit code (recording bgbash-failed).
// sr:proves task-stream-frames/claude
// sr:proves task-notifications/claude
func TestT017_112_AFailedBackgroundShellIsReportedFailed(t *testing.T) {
	summaries := func(frames []map[string]any) (out []string) {
		for _, f := range frames {
			switch f["subtype"] {
			case "task_updated":
				patch := f["patch"].(map[string]any)
				assert.Contains(t, patch, "end_time", "the patch says when the task ended")
				out = append(out, "updated:"+patch["status"].(string))
			case "task_notification":
				out = append(out, f["status"].(string)+":"+f["summary"].(string))
			}
		}
		return out
	}
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgbash-failed/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := summaries(streamFrames(t, string(data)))
	require.Equal(t, []string{"updated:failed", `failed:Background command "failing" failed with exit code 3`}, want, "recorded")

	dir := t.TempDir()
	sc := script(t, dir, "s",
		toolUse("bg1", "Bash", `{"command":"sleep 0.2; exit 3","description":"failing","run_in_background":true}`),
		toolUse("fg1", "Bash", `{"command":"sleep 1"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bf-9", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, summaries(streamFrames(t, out)))
}

// startedOwnedBySubagent is the owned_by_subagent flag of each local_bash task_started frame, in order.
func startedOwnedBySubagent(frames []map[string]any) (out []any) {
	for _, f := range frames {
		if f["subtype"] == "task_started" && f["task_type"] == "local_bash" {
			out = append(out, f["owned_by_subagent"])
		}
	}
	return out
}

// TestT017_117_ABackgroundShellOfASubagentIsMarkedOwned: the task_started of a background Bash that a
// sub-agent launched says owned_by_subagent, which the main thread's own does not (recording
// fg-subagent-bash).
// sr:proves task-stream-frames/claude
func TestT017_117_ABackgroundShellOfASubagentIsMarkedOwned(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/fg-subagent-bash/samples/*/stream.jsonl"))
	require.NoError(t, err)
	require.Equal(t, []any{true}, startedOwnedBySubagent(streamFrames(t, string(data))), "recorded")

	dir := t.TempDir()
	sub := script(t, dir, "sub", toolUse("bg1", "Bash", `{"command":"sleep 30","description":"bgsleep","run_in_background":true}`))
	orch := script(t, dir, "orch", toolUse("ag1", "Agent", `{"prompt":"p","description":"d","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "os-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []any{true}, startedOwnedBySubagent(streamFrames(t, out)))
}
